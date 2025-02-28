package com.github.bazel_contrib.target_determinator.integration;

import com.google.devtools.build.runfiles.Runfiles;
import org.eclipse.jgit.api.AddCommand;
import org.eclipse.jgit.api.Git;
import org.eclipse.jgit.api.errors.GitAPIException;
import org.eclipse.jgit.lib.Constants;
import org.eclipse.jgit.lib.Repository;
import org.eclipse.jgit.revwalk.RevCommit;
import org.eclipse.jgit.storage.file.FileBasedConfig;
import org.eclipse.jgit.storage.file.FileRepositoryBuilder;

import java.io.File;
import java.io.IOException;
import java.io.UncheckedIOException;
import java.net.URI;
import java.nio.file.FileVisitResult;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.nio.file.SimpleFileVisitor;
import java.nio.file.attribute.BasicFileAttributes;
import java.util.Comparator;
import java.util.Objects;
import java.util.Set;
import java.util.function.Predicate;
import java.util.stream.Stream;

import static java.nio.file.FileVisitOption.FOLLOW_LINKS;
import static java.nio.file.StandardCopyOption.COPY_ATTRIBUTES;
import static java.nio.file.StandardCopyOption.REPLACE_EXISTING;
import static org.junit.Assert.assertTrue;
import static org.junit.Assert.fail;

public class TestRepo {

    private final Path dir;
    private final Git gitRepo;

    private TestRepo(Path path, Git gitRepo) {
        this.dir = Objects.requireNonNull(path);
        this.gitRepo = Objects.requireNonNull(gitRepo);
    }

    public static TestRepo create(Path dir) {
        assertTrue(Files.exists(dir));

        try {
            Git gitRepo = getGitRepo(dir);

            return new TestRepo(dir, gitRepo);
        } catch (GitAPIException | IOException e) {
            throw new RuntimeException(e);
        }
    }

    private static Git getGitRepo(Path dir) throws GitAPIException, IOException {
        Git git;

        if (Files.exists(dir.resolve(".git"))) {
            Repository repo = new FileRepositoryBuilder()
                    .setGitDir(dir.resolve(".git").toFile())
                    .setInitialBranch("main")
                    .build();
            git = new Git(repo);
        } else {
            git = Git.init().setDirectory(dir.toFile()).setInitialBranch("main").call();
        }

        // By default, remote repos are not allowed to use the `file`
        // URI scheme for things like submodules. That's unfortunate
        // since that's exactly what we need. Override that setting.
        FileBasedConfig config = (FileBasedConfig) git.getRepository().getConfig();
        config.setString("protocol", "file", "allow", "always");
        config.save();

        return git;
    }

    public Path getDir() {
        return dir;
    }

    public URI getUri() {
        return dir.toUri();
    }

    public String commit(String message, String... additionalPaths) {
        try {
            AddCommand add = gitRepo.add().addFilepattern(".").addFilepattern(Constants.DOT_GIT_MODULES);
            for (String additionalPath : additionalPaths) {
                add.addFilepattern(additionalPath);
            }
            add.call();

            RevCommit revCommit = gitRepo.commit()
                    .setAll(true)
                    .setMessage(message)
                    .call();
            return revCommit.getId().getName();
        } catch (GitAPIException e) {
            throw new RuntimeException(e);
        }
    }

    public void replaceWithContentsFrom(String pathWithinTestData) {
        try {
            Runfiles runfiles = Runfiles.preload().unmapped();

            String packageAsPath = getClass().getPackageName().replace(".", File.separator);

            String rlocation = runfiles.rlocation("target-determinator/tests/integration/java/" + packageAsPath + "/testdata/" + pathWithinTestData);

            Path directory = Paths.get(rlocation);
            if (!Files.exists(directory)) {
                fail("Unable to find " + pathWithinTestData);
            }
            if (!Files.isDirectory(directory)) {
                fail("Found path, but it is not a directory: " + rlocation);
            }

            // First delete everything other than the `.git` directory
            deleteExcept(p -> {
                // We want to leave the entire `.git` directory alone
                for (Path segment : p) {
                    if (segment.toString().equals(".git")) {
                        return false;
                    }
                }
                return true;
            });
            copyRecursively(directory, dir);
        } catch (IOException e) {
            throw new UncheckedIOException(e);
        }
    }

    private void copyRecursively(Path source, Path destination) throws IOException {
        Files.walkFileTree(
                source,
                Set.of(FOLLOW_LINKS),
                Integer.MAX_VALUE,
                new SimpleFileVisitor<>() {
                    @Override
                    public FileVisitResult preVisitDirectory(Path dir, BasicFileAttributes attrs) throws IOException {
                        Path targetDir = destination.resolve(source.relativize(dir));
                        if (!Files.exists(targetDir)) {
                            Files.createDirectories(targetDir);
                        }
                        return FileVisitResult.CONTINUE;
                    }

                    @Override
                    public FileVisitResult visitFile(Path file, BasicFileAttributes attrs) throws IOException {
                        Path targetFile = destination.resolve(source.relativize(file));
                        if ("BUILD.bazel.mv".equals(targetFile.getFileName().toString())) {
                            targetFile = targetFile.getParent().resolve("BUILD.bazel");
                        }
                        Files.copy(file, targetFile, COPY_ATTRIBUTES, REPLACE_EXISTING);
                        return FileVisitResult.CONTINUE;
                    }
                });
    }

    private void deleteExcept(Predicate<Path> pathsThatMatchThis) {
        try (Stream<Path> paths = Files.walk(dir)) {
            paths.sorted(Comparator.reverseOrder())
                    .filter(p -> pathsThatMatchThis.test(dir.relativize(p)))
                    .filter(p -> !dir.equals(p))
                    .map(Path::toFile)
                    .forEach(File::delete);
        } catch (IOException e) {
            throw new UncheckedIOException(e);
        }
    }

    // We allow the underlying API to poke through here because it makes life easy
    public TestRepo addSubModule(TestRepo submodule, String pathInLocalRepo) {
        try {
            Repository repo = gitRepo.submoduleAdd()
                    .setURI(submodule.getUri().toString())
                    .setPath(pathInLocalRepo)
                    .call();

            return new TestRepo(dir.resolve(pathInLocalRepo), new Git(repo));
        } catch (GitAPIException e) {
            throw new RuntimeException(e);
        }
    }

    public void pull() {
        try {
            var res = gitRepo.pull()
                    .setRemote("origin")
                    .setRemoteBranchName("main")
                    .call();
            assertTrue(res.isSuccessful());
        } catch (GitAPIException e) {
            throw new RuntimeException(e);
        }
    }

    public void move(String from, String to) {
        try {
            Files.move(dir.resolve(from), dir.resolve(to));
        } catch (IOException e) {
            throw new UncheckedIOException(e);
        }
    }

    public String createBranch(String branchName) {
        try {
            gitRepo.checkout().setCreateBranch(true).setName(branchName).call();

            return getBranch();
        } catch (GitAPIException e) {
            throw new RuntimeException(e);
        }
    }

    public String getBranch() {
        try {
            return gitRepo.getRepository().getBranch();
        } catch (IOException e) {
            throw new UncheckedIOException(e);
        }
    }
}

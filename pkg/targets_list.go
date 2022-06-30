package pkg

type TargetsList struct {
	targets string
}

func ParseTargetsList(targets string) (TargetsList, error) {
	// TODO: validate against syntax in https://bazel.build/reference/query
	return TargetsList{targets: targets}, nil
}

func (tl *TargetsList) String() string {
	return tl.targets
}

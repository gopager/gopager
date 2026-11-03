package processing

import (
	"encoding/json"
	"errors"
	"slices"
)

type dependencyKey struct {
	transmission string
	source       int64
	context      int64
	network      string
	target       bool
	position     uint32
}

func (d Dependency) key() dependencyKey {
	key := dependencyKey{transmission: d.TransmissionID.String(), source: d.SourceRevision,
		context: d.ContextRevision, network: d.NetworkID.String(), target: d.TargetPosition != nil}
	if d.TargetPosition != nil {
		key.position = *d.TargetPosition
	}
	return key
}

// UnionDependencies validates and combines classification sets without losing distinct
// source revisions or target contexts. Its returned slices/pointers do not alias inputs.
func UnionDependencies(sets ...[]Dependency) ([]Dependency, error) {
	indexes := make(map[dependencyKey]int)
	var result []Dependency
	encodedSize := 0
	for _, set := range sets {
		for _, dependency := range set {
			if err := dependency.validate(); err != nil {
				return nil, err
			}
			encoded, err := json.Marshal(dependency)
			if err != nil {
				return nil, invalid(err)
			}
			encodedSize += len(encoded)
			if encodedSize > MaxInputBytes {
				return nil, resource(errors.New("dependency input boundary"))
			}
			key := dependency.key()
			index, exists := indexes[key]
			if !exists {
				index = len(result)
				indexes[key] = index
				dependency.Groups = slices.Clone(dependency.Groups)
				if dependency.TargetPosition != nil {
					position := *dependency.TargetPosition
					dependency.TargetPosition = &position
				}
				result = append(result, dependency)
			} else {
				result[index].Groups = append(result[index].Groups, dependency.Groups...)
			}
		}
	}
	for i := range result {
		slices.Sort(result[i].Groups)
		result[i].Groups = slices.Compact(result[i].Groups)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, invalid(err)
	}
	if len(encoded) > MaxInputBytes {
		return nil, resource(errors.New("dependency output boundary"))
	}
	return result, nil
}

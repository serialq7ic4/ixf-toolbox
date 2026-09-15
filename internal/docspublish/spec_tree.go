package docspublish

import "fmt"

func walkSpecs(specs []Spec, visit func(Spec)) {
	for _, spec := range specs {
		visit(spec)
		walkSpecs(spec.Children, visit)
	}
}

func countNestedSpecs(specs []Spec) int {
	count := 0
	walkSpecs(specs, func(spec Spec) {
		count += len(spec.Children)
	})
	return count
}

func validateSpecTree(specs []Spec) error {
	for _, spec := range specs {
		if len(spec.Children) > 0 && spec.Kind != "ordered" {
			return fmt.Errorf("spec kind %q cannot contain children", spec.Kind)
		}
		if err := validateSpecTree(spec.Children); err != nil {
			return err
		}
	}
	return nil
}

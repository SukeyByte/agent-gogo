package contextbuilder

type LayerInvalidation struct {
	Layer       LayerName
	PreviousKey string
	CurrentKey  string
	Reason      string
}

type BlockInvalidation struct {
	Layer       LayerName
	Block       string
	PreviousKey string
	CurrentKey  string
}

func DiffLayerKeys(previous, current ContextLayerKeys) []LayerInvalidation {
	candidates := []LayerInvalidation{
		{
			Layer:       LayerL0,
			PreviousKey: previous.L0,
			CurrentKey:  current.L0,
			Reason:      "runtime rules, security rules, or active personas changed",
		},
		{
			Layer:       LayerL1,
			PreviousKey: previous.L1,
			CurrentKey:  current.L1,
			Reason:      "project route inputs, channel capabilities, active capabilities, functions, or skills changed",
		},
		{
			Layer:       LayerL2,
			PreviousKey: previous.L2,
			CurrentKey:  current.L2,
			Reason:      "intent profile, project state, task state, relevant memories, or acceptance criteria changed",
		},
		{
			Layer:       LayerL3,
			PreviousKey: previous.L3,
			CurrentKey:  current.L3,
			Reason:      "dynamic evidence, recent messages, or current user input changed",
		},
	}

	result := make([]LayerInvalidation, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.PreviousKey != candidate.CurrentKey {
			result = append(result, candidate)
		}
	}
	return result
}

func StablePrefix(previous, current ContextLayerKeys) []LayerName {
	ordered := []struct {
		name LayerName
		prev string
		next string
	}{
		{LayerL0, previous.L0, current.L0},
		{LayerL1, previous.L1, current.L1},
		{LayerL2, previous.L2, current.L2},
		{LayerL3, previous.L3, current.L3},
	}

	result := make([]LayerName, 0, len(ordered))
	for _, layer := range ordered {
		if layer.prev != layer.next {
			break
		}
		result = append(result, layer.name)
	}
	return result
}

func DiffBlockKeys(previous, current ContextBlockKeys) []BlockInvalidation {
	result := []BlockInvalidation{}
	result = append(result, diffBlockMap(LayerL0, previous.L0, current.L0)...)
	result = append(result, diffBlockMap(LayerL1, previous.L1, current.L1)...)
	result = append(result, diffBlockMap(LayerL2, previous.L2, current.L2)...)
	result = append(result, diffBlockMap(LayerL3, previous.L3, current.L3)...)
	return result
}

func diffBlockMap(layer LayerName, previous, current map[string]string) []BlockInvalidation {
	names := map[string]struct{}{}
	for name := range previous {
		names[name] = struct{}{}
	}
	for name := range current {
		names[name] = struct{}{}
	}

	result := make([]BlockInvalidation, 0, len(names))
	for name := range names {
		if previous[name] == current[name] {
			continue
		}
		result = append(result, BlockInvalidation{
			Layer:       layer,
			Block:       name,
			PreviousKey: previous[name],
			CurrentKey:  current[name],
		})
	}
	sortBlockInvalidations(result)
	return result
}

func sortBlockInvalidations(values []BlockInvalidation) {
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j].Layer < values[i].Layer || values[j].Layer == values[i].Layer && values[j].Block < values[i].Block {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}

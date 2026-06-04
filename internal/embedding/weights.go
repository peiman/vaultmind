package embedding

import (
	"fmt"
	"os"

	"github.com/nlpodyssey/gopickle/pytorch"
	"github.com/nlpodyssey/gopickle/types"
)

// LoadLinearWeights loads a PyTorch nn.Linear layer's weight and bias from a .pt file.
// Returns weight as [out_features][in_features] and bias as [out_features].
// The .pt file must be a state_dict saved via torch.save(state_dict, path).
func LoadLinearWeights(path string) (weight [][]float32, bias []float32, err error) {
	if _, statErr := os.Stat(path); statErr != nil {
		return nil, nil, fmt.Errorf("weight file not found: %w", statErr)
	}

	result, loadErr := pytorch.Load(path)
	if loadErr != nil {
		return nil, nil, fmt.Errorf("loading PyTorch file %q: %w", path, loadErr)
	}

	stateDict, ok := result.(*types.OrderedDict)
	if !ok {
		return nil, nil, fmt.Errorf("expected OrderedDict from %q, got %T", path, result)
	}

	for el := stateDict.List.Front(); el != nil; el = el.Next() {
		entry := el.Value.(*types.OrderedDictEntry)
		name, _ := entry.Key.(string)
		tensor, ok := entry.Value.(*pytorch.Tensor)
		if !ok {
			continue
		}
		data, dataErr := float32Data(tensor.Source)
		if dataErr != nil {
			return nil, nil, fmt.Errorf("tensor %q: %w", name, dataErr)
		}

		switch name {
		case "weight":
			if len(tensor.Size) != 2 {
				return nil, nil, fmt.Errorf("weight tensor has %d dims, expected 2", len(tensor.Size))
			}
			outFeatures := tensor.Size[0]
			inFeatures := tensor.Size[1]
			weight = make([][]float32, outFeatures)
			for i := 0; i < outFeatures; i++ {
				weight[i] = make([]float32, inFeatures)
				copy(weight[i], data[i*inFeatures:(i+1)*inFeatures])
			}
		case "bias":
			bias = make([]float32, len(data))
			copy(bias, data)
		}
	}

	if weight == nil {
		return nil, nil, fmt.Errorf("no 'weight' tensor found in %q", path)
	}
	return weight, bias, nil
}

// float32Data extracts []float32 from a PyTorch storage.
// Supports FloatStorage, HalfStorage, and BFloat16Storage (all store Data as []float32).
func float32Data(source pytorch.StorageInterface) ([]float32, error) {
	switch s := source.(type) {
	case *pytorch.FloatStorage:
		return s.Data, nil
	case *pytorch.HalfStorage:
		return s.Data, nil
	case *pytorch.BFloat16Storage:
		return s.Data, nil
	default:
		return nil, fmt.Errorf("unsupported storage type %T, expected float32-compatible storage", source)
	}
}

package geometry

import (
	"errors"
	"fmt"
)

// maxLayers caps the layers of one element: each layer is a mesh of its own.
const maxLayers = 16

// checkLayers refuses a layer list that is empty, longer than maxLayers, or
// holds a thickness out of bounds, and returns the summed thickness.
func checkLayers(layers []float64) (total float64, err error) {
	if len(layers) == 0 {
		return 0, errors.New("no layers: a layered element needs at least one layer")
	}
	if len(layers) > maxLayers {
		return 0, fmt.Errorf("%d layers, more than %d", len(layers), maxLayers)
	}
	for k, t := range layers {
		if err := checkDim(fmt.Sprintf("layer %d: thickness", k), t); err != nil {
			return 0, err
		}
		total += t
	}
	return total, checkDim("thickness", total)
}

// BuildSlabLayers builds a slab in layers, top first: layer k's top face lies
// at z minus the layers above it. Each layer is a Slab with its own volume;
// all share the outline's area and perimeter.
// Invalid input returns an error and nothing else.
func BuildSlabLayers(outline [][2]float64, z float64, layers []float64) ([]Slab, error) {
	if err := checkOutline(outline); err != nil {
		return nil, err
	}
	if err := checkOrigin("z", z); err != nil {
		return nil, err
	}
	if _, err := checkLayers(layers); err != nil {
		return nil, err
	}
	ring, cap, err := slabCap(outline)
	if err != nil {
		return nil, err
	}
	slabs := make([]Slab, len(layers))
	top := z
	for k, t := range layers {
		slabs[k] = slabAt(outline, ring, cap, top, t)
		top -= t
	}
	return slabs, nil
}

package model

import (
	"sort"

	"github.com/blox-eng/common/ifc/step"
)

// emitOrRenderClasses is the complete set of concrete IfcElement subtype names
// across IFC2X3 and IFC4 (union, generated from the ifcopenshell schema —
// ifcopenshell.ifcopenshell_wrapper.schema_by_name(...).declaration_by_name("IfcElement")
// walked recursively via .subtypes()), minus IfcOpeningElement/IfcOpeningStandardCase
// (the oracle's by_type("IfcElement") selection excludes openings — they are voids,
// not real elements). This matches the oracle's selection semantics: ifcopenshell's
// by_type() expands to all schema subtypes automatically, but the step layer is
// exact-match only (step.ByType does no schema subtype expansion), so the full
// subtype closure must be baked in here instead. Every entry below is provably an
// IfcElement subtype by construction, so this list can only ever be under-inclusive
// relative to the oracle (a missing element class), never over-inclusive (an EXTRA
// class the oracle wouldn't also emit).
var emitOrRenderClasses = []string{
	"IfcActuator", "IfcAirTerminal", "IfcAirTerminalBox",
	"IfcAirToAirHeatRecovery", "IfcAlarm", "IfcAudioVisualAppliance",
	"IfcBeam", "IfcBeamStandardCase", "IfcBoiler", "IfcBuildingElementPart",
	"IfcBuildingElementProxy", "IfcBurner", "IfcCableCarrierFitting",
	"IfcCableCarrierSegment", "IfcCableFitting", "IfcCableSegment",
	"IfcChamferEdgeFeature", "IfcChiller", "IfcChimney", "IfcCivilElement",
	"IfcCoil", "IfcColumn", "IfcColumnStandardCase",
	"IfcCommunicationsAppliance", "IfcCompressor", "IfcCondenser",
	"IfcController", "IfcCooledBeam", "IfcCoolingTower", "IfcCovering",
	"IfcCurtainWall", "IfcDamper", "IfcDiscreteAccessory",
	"IfcDistributionChamberElement", "IfcDistributionControlElement",
	"IfcDistributionElement", "IfcDistributionFlowElement", "IfcDoor",
	"IfcDoorStandardCase", "IfcDuctFitting", "IfcDuctSegment",
	"IfcDuctSilencer", "IfcElectricAppliance", "IfcElectricDistributionBoard",
	"IfcElectricDistributionPoint", "IfcElectricFlowStorageDevice",
	"IfcElectricGenerator", "IfcElectricMotor", "IfcElectricTimeControl",
	"IfcElectricalElement", "IfcElementAssembly", "IfcEnergyConversionDevice",
	"IfcEngine", "IfcEquipmentElement", "IfcEvaporativeCooler",
	"IfcEvaporator", "IfcFan", "IfcFastener", "IfcFilter",
	"IfcFireSuppressionTerminal", "IfcFlowController", "IfcFlowFitting",
	"IfcFlowInstrument", "IfcFlowMeter", "IfcFlowMovingDevice",
	"IfcFlowSegment", "IfcFlowStorageDevice", "IfcFlowTerminal",
	"IfcFlowTreatmentDevice", "IfcFooting", "IfcFurnishingElement",
	"IfcFurniture", "IfcGeographicElement", "IfcHeatExchanger",
	"IfcHumidifier", "IfcInterceptor", "IfcJunctionBox", "IfcLamp",
	"IfcLightFixture", "IfcMechanicalFastener", "IfcMedicalDevice",
	"IfcMember", "IfcMemberStandardCase", "IfcMotorConnection", "IfcOutlet",
	"IfcPile", "IfcPipeFitting", "IfcPipeSegment", "IfcPlate",
	"IfcPlateStandardCase", "IfcProjectionElement", "IfcProtectiveDevice",
	"IfcProtectiveDeviceTrippingUnit", "IfcPump", "IfcRailing", "IfcRamp",
	"IfcRampFlight", "IfcReinforcingBar", "IfcReinforcingMesh", "IfcRoof",
	"IfcRoundedEdgeFeature", "IfcSanitaryTerminal", "IfcSensor",
	"IfcShadingDevice", "IfcSlab", "IfcSlabElementedCase",
	"IfcSlabStandardCase", "IfcSolarDevice", "IfcSpaceHeater",
	"IfcStackTerminal", "IfcStair", "IfcStairFlight", "IfcSurfaceFeature",
	"IfcSwitchingDevice", "IfcSystemFurnitureElement", "IfcTank",
	"IfcTendon", "IfcTendonAnchor", "IfcTransformer", "IfcTransportElement",
	"IfcTubeBundle", "IfcUnitaryControlElement", "IfcUnitaryEquipment",
	"IfcValve", "IfcVibrationIsolator", "IfcVirtualElement",
	"IfcVoidingFeature", "IfcWall", "IfcWallElementedCase",
	"IfcWallStandardCase", "IfcWasteTerminal", "IfcWindow",
	"IfcWindowStandardCase",
}

// Extract walks the file into semantic []Element, ports decompose.decompose's
// non-geometry half. Placement is scaled to meters; quantities are tier-1 Qto with
// provenance ("qto"|"none"). ParentIndex indexes into Result.Elements (spatial
// parent).
func Extract(f *step.File) (*Result, error) {
	scale := UnitScale(f)
	res := &Result{UnitScale: scale}
	if UnitIsUnhandled(f) {
		res.Warnings = append(res.Warnings, "length unit is not a recognized SI unit (imperial/conversion-based?); placements/quantities assume meters (scale=1.0)")
	}

	// gather elements in stable source order
	var insts []*step.Instance
	seen := map[int]bool{}
	for _, cls := range emitOrRenderClasses {
		for _, e := range f.ByType(cls) {
			if seen[e.ID()] {
				continue
			}
			seen[e.ID()] = true
			insts = append(insts, e)
		}
	}
	sortByID(insts) // deterministic order

	idxByID := map[int]int{}
	for i, e := range insts {
		idxByID[e.ID()] = i
	}

	for _, e := range insts {
		m := LocalPlacement(e)
		m[12], m[13], m[14] = m[12]*scale, m[13]*scale, m[14]*scale

		q, hasQto := QtoQuantities(f, e, scale)
		src := QuantitySourceQto
		if !hasQto {
			// No silent partial: source="none" always pairs with a fully-empty
			// Qto. The geometry tier back-fills these post-Build via
			// ApplyDerivedQuantities (a "none" element with a mesh becomes
			// "geometry"); no per-element warning here — the source tag is the signal.
			q = Quantities{}
			src = QuantitySourceNone
		}

		var material string
		for _, mat := range Materials(f, e) {
			if name := strVal(mat, 0); name != "" {
				material = name
				break
			}
		}

		var parent *int
		if c := Container(f, e); c != nil {
			if pi, ok := idxByID[c.ID()]; ok {
				parent = &pi
			}
		}

		res.Elements = append(res.Elements, Element{
			GlobalID:       strVal(e, attrGlobalID),
			ExpressID:      e.ID(),
			IFCClass:       e.Type(),
			Name:           strVal(e, attrName),
			PredefinedType: predefinedType(f, e),
			Category:       Category(f, e),
			Storey:         Storey(f, e),
			Material:       material,
			IsExternal:     IsExternal(f, e),
			Psets:          Psets(f, e, false),
			Qto:            q,
			QuantitySource: src,
			Placement:      m,
			ParentIndex:    parent,
			Emit:           isStructural(e.Type()),
		})
	}
	return res, nil
}

// sortByID sorts insts ascending by ID() for deterministic output regardless of
// map/ByType iteration order.
func sortByID(insts []*step.Instance) {
	sort.Slice(insts, func(i, j int) bool { return insts[i].ID() < insts[j].ID() })
}

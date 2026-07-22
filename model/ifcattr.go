package model

// Well-known IFC attribute indices (0-based, positional). Stable across IFC2X3
// and IFC4 for these core entities. Each constant names the entity + its full
// attribute list so the index is auditable without the EXPRESS schema.

// IfcProduct: [GlobalId,OwnerHistory,Name,Description,ObjectType,ObjectPlacement,Representation]
const (
	attrGlobalID        = 0
	attrName            = 2
	attrObjectPlacement = 5
)

// IfcLocalPlacement: [PlacementRelTo, RelativePlacement]
const (
	attrPlacementRelTo    = 0
	attrRelativePlacement = 1
)

// IfcAxis2Placement3D: [Location, Axis(Z), RefDirection(X)]
const (
	attrAxisLocation = 0
	attrAxisZ        = 1
	attrAxisX        = 2
)

// IfcCartesianPoint: [Coordinates]; IfcDirection: [DirectionRatios]
const attrCoordinates = 0

// Rel* entities: [GlobalId,OwnerHistory,Name,Description, Rel4, Rel5]
const (
	attrRel4 = 4 // RelatedObjects / RelatedElements
	attrRel5 = 5 // RelatingStructure / RelatingObject / RelatingPropertyDefinition / RelatingType / RelatingMaterial
)

// IfcPropertySet: [GlobalId,OwnerHistory,Name,Description,HasProperties]
const attrHasProperties = 4

// IfcElementQuantity: [GlobalId,OwnerHistory,Name,Description,MethodOfMeasurement,Quantities]
const attrQuantities = 5

// IfcPropertySingleValue: [Name,Description,NominalValue,Unit]
const (
	attrPropName     = 0
	attrNominalValue = 2
)

// IfcTypeObject: [GlobalId,OwnerHistory,Name,Description,ApplicableOccurrence,
// HasPropertySets]. Stable across subtypes (IfcTypeProduct/IfcElementType/
// IfcWallType/...) since subtype attributes append after the base's.
const attrHasPropertySets = 5

// PredefinedType's positional attribute index varies by (schema, class); see
// predefinedTypeIndexIFC4 / predefinedTypeIndexIFC2X3 in category.go.

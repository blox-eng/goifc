//go:build race

package geometry

// raceBuild is true under -race, whose instrumentation slows the meshing loops
// (see timeLimit).
const raceBuild = true

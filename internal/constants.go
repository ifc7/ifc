package internal

import "path"

const (
	IfcFolder       = ".ifc"
	IfcConfigFile   = "ifc.yaml"
	IfcManifestFile = "manifest.json"

	DefaultAPIHost         = "staging.ifc7.dev"
	DefaultAPIURL          = "https://" + DefaultAPIHost + "/api/v0/"
	DeviceFlowClientId     = "4d14om5n01t8f29kja086002re"
	DeviceFlowClientSecret = "" // no secret used
	DeviceFlowURL          = "https://device-auth.staging.ifc7.dev"
	CognitoDomain          = "ifc7-device-auth.auth.us-east-1.amazoncognito.com"
)

var (
	IfcManifestPath = path.Join(IfcFolder, IfcManifestFile)
)

var (
	BuildVersion = "dev"
	GitCommit    = "none"
	BuildTime    = "unknown"
)

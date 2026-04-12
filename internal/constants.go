package internal

import "path"

const (
	IfcFolder          = ".ifc"
	IfcConfigFile      = "ifc.yaml"
	IfcManifestFile    = "manifest.json"
	IfcCredentialsFile = "credentials.json"

	DefaultAPIURL          = "https://dev.ifc7.dev/api/v0/"
	DefaultAPIHost         = "dev.ifc7.dev"
	DeviceFlowClientId     = "4d14om5n01t8f29kja086002re"
	DeviceFlowClientSecret = "" // no secret used
	DeviceFlowURL          = "https://device-auth.nonprod.ifc7.dev"
	CognitoDomain          = "ifc7-device-auth.auth.us-east-1.amazoncognito.com"
)

var (
	IfcManifestPath    = path.Join(IfcFolder, IfcManifestFile)
	IfcCredentialsPath = path.Join(IfcFolder, IfcCredentialsFile)
)

var (
	BuildVersion = "dev"
	GitCommit    = "none"
	BuildTime    = "unknown"
)

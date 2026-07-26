package internal

import "path"

const (
	IfcFolder       = ".ifc"
	IfcConfigFile   = "ifc.yaml"
	IfcManifestFile = "manifest.json"

	DefaultAPIHost     = "ifc7.dev"
	DefaultAPIURL      = "https://" + DefaultAPIHost + "/api/v0/"
	DeviceFlowClientId = "4ul2l9rbkt7p9g13ho2cnf0pgf"
	DeviceFlowURL      = "https://device-auth.ifc7.dev"
	CognitoDomain      = "https://ifc7-device-auth.auth.us-east-1.amazoncognito.com"
)

var (
	IfcManifestPath = path.Join(IfcFolder, IfcManifestFile)
)

var (
	BuildVersion = "dev"
	GitCommit    = "none"
	BuildTime    = "unknown"
)

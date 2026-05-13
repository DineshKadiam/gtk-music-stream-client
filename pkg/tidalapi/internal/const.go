package internal

import "encoding/base64"

const (
	clientIDPart1     = "V1hwNFJFWmFOMU5G"
	clientIDPart2     = "U2tabmNVNUplZz09"
	clientSecretPart1 = "YkRGTGVYbHdhMmg0U0hKRlRqbFNRMU4zVlZkU1ow"
	clientSecretPart2 = "OXNWa3BVYzFWS1NsazJka2x4VnpoSlprOXRZejA9"

	ClientVersion           = "2025.12.13"
	UnauthenticatedClientID = "txNoH4kkV41MfH25"
)

var (
	ClientID     string
	ClientSecret string
)

func init() {
	clientIdPart1Decoded, _ := base64.StdEncoding.DecodeString(clientIDPart1)
	clientIdPart2Decoded, _ := base64.StdEncoding.DecodeString(clientIDPart2)
	clientIdDecoded, _ := base64.StdEncoding.DecodeString(string(clientIdPart1Decoded) + string(clientIdPart2Decoded))
	ClientID = string(clientIdDecoded)

	clientSecretPart1Decoded, _ := base64.StdEncoding.DecodeString(clientSecretPart1)
	clientSecretPart2Decoded, _ := base64.StdEncoding.DecodeString(clientSecretPart2)
	clientSecretDecoded, _ := base64.StdEncoding.DecodeString(string(clientSecretPart1Decoded) + string(clientSecretPart2Decoded))
	ClientSecret = string(clientSecretDecoded)
}

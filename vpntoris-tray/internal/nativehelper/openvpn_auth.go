package nativehelper

import (
	"encoding/base64"
	"fmt"
	"net"
	"strings"
)

func writeManagementCredentials(connection net.Conn, username string, password string) error {
	_, err := fmt.Fprintf(connection, "username \"Auth\" \"%s\"\npassword \"Auth\" \"%s\"\n", managementEscape(username), managementEscape(password))
	return err
}
func openVPNChallengeCredentials(challenge string, state string, username string, password string, otp string) (string, string) {
	switch challenge {
	case "static":
		password = "SCRV1:" + base64.StdEncoding.EncodeToString([]byte(password)) + ":" + base64.StdEncoding.EncodeToString([]byte(otp))
	case "dynamic":
		password = "CRV1::" + state + "::" + otp
	case "append":
		password += otp
	}
	return username, password
}
func managementEscape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

package nativehelper

import (
	"fmt"
	"strings"
)

func buildCharonConfiguration(pluginDir, viciSocket, pidFile string) string {
	configuration := fmt.Sprintf("libstrongswan {\n  plugin_dir = %s\n}\ncharon {\n  install_routes = yes\n  load_modular = yes\n  plugins {\n    random { load = yes }\n    nonce { load = yes }\n    x509 { load = yes }\n    revocation { load = yes }\n    constraints { load = yes }\n    pubkey { load = yes }\n    pkcs1 { load = yes }\n    pkcs8 { load = yes }\n    pem { load = yes }\n    openssl { load = yes }\n%s    socket-default { load = yes }\n    vici {\n      load = yes\n      socket = unix://%s\n    }\n    eap-identity { load = yes }\n    eap-mschapv2 { load = yes }\n    eap-md5 { load = yes }\n    eap-gtc { load = yes }\n    eap-peap { load = yes }\n    xauth-generic { load = yes }\n    unity { load = yes }\n    counters { load = yes }\n    curve25519 { load = yes }\n    kdf { load = yes }\n  }\n}\n", pluginDir, charonKernelPlugins(), viciSocket)
	return strings.Replace(configuration, "charon {\n", fmt.Sprintf("charon {\n  pid_file = %s\n", pidFile), 1)
}

//go:build unix

package helperipc

import (
	"bufio"
	"encoding/json"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"vpntoris-tray/internal/fortihelper"
	"vpntoris-tray/internal/nativehelper"
	"vpntoris-tray/internal/runtimepaths"
)

// AllowedUsersPath lists the local uids allowed to drive the root helper when
// the socket cannot be owned by a single desktop user (Linux systemd installs,
// where the helper starts before any user session exists). Package postinst
// scripts record the installing desktop user here.
const AllowedUsersPath = "/etc/vpntoris/helper-users.conf"

var warnAllowAllOnce sync.Once

// ServeUnix starts a Unix-domain control socket for the helper service.
//
// Access model:
//   - uid > 0 (macOS LaunchDaemon passes the console user): the socket is
//     chowned to that uid with mode 0660, so only root and the desktop user
//     can connect.
//   - uid == 0 (Linux systemd): the socket stays root:root 0666, but every
//     connection is checked via SO_PEERCRED against AllowedUsersPath. When
//     the allow file is absent (dev setups) all local users are accepted and
//     a warning is logged once.
func ServeUnix(service *nativehelper.Service, paths runtimepaths.Paths, uid int) error {
	socketPath := paths.HelperSocket
	if err := os.MkdirAll(filepath.Dir(socketPath), 0755); err != nil {
		return err
	}
	_ = os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	if uid > 0 {
		if err := os.Chown(socketPath, uid, -1); err != nil {
			listener.Close()
			return err
		}
		if err := os.Chmod(socketPath, 0660); err != nil {
			listener.Close()
			return err
		}
	} else {
		if err := os.Chmod(socketPath, 0666); err != nil {
			listener.Close()
			return err
		}
	}
	for {
		connection, err := listener.Accept()
		if err != nil {
			continue
		}
		if uid == 0 && !peerAllowed(connection) {
			_ = connection.Close()
			continue
		}
		go handle(service, connection)
	}
}

// peerAllowed reports whether the connecting peer may issue helper requests.
// Root is always allowed; other uids must appear in AllowedUsersPath. A
// missing/unreadable allow file keeps legacy behaviour (allow all) so dev and
// smoke setups keep working, with a single warning.
func peerAllowed(connection net.Conn) bool {
	peerUID, err := peerCredentialsUID(connection)
	if err != nil {
		// Cannot identify the peer: fail closed.
		log.Printf("helper: rejecting connection, peer credentials unavailable: %v", err)
		return false
	}
	if peerUID == 0 {
		return true
	}
	allowed, err := loadAllowedUsers()
	if err != nil || len(allowed) == 0 {
		warnAllowAllOnce.Do(func() {
			log.Printf("helper: %s missing or empty; allowing all local users (install the package to restrict)", AllowedUsersPath)
		})
		return true
	}
	if !allowed[peerUID] {
		log.Printf("helper: rejecting request from uid %d (not in %s)", peerUID, AllowedUsersPath)
		return false
	}
	return true
}

// loadAllowedUsers parses AllowedUsersPath (one uid per line, # comments).
// Read per connection so postinst updates apply without a helper restart.
func loadAllowedUsers() (map[uint32]bool, error) {
	file, err := os.Open(AllowedUsersPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	allowed := map[uint32]bool{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		uid, err := strconv.ParseUint(line, 10, 32)
		if err != nil {
			continue
		}
		allowed[uint32(uid)] = true
	}
	return allowed, scanner.Err()
}

func handle(service *nativehelper.Service, connection net.Conn) {
	defer connection.Close()
	decoder := json.NewDecoder(connection)
	decoder.DisallowUnknownFields()
	var request fortihelper.Request
	if err := decoder.Decode(&request); err != nil {
		_ = json.NewEncoder(connection).Encode(fortihelper.Response{State: "failed", Error: "invalid request"})
		return
	}
	_ = json.NewEncoder(connection).Encode(service.Handle(request))
}

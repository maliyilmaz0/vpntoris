# VPNToris — Agent Project Guide

Bu dosya, projeye yeni katılan bir agent’ın kodu hızlı ve güvenli biçimde anlayabilmesi için hazırlanmıştır. Kullanıcıya yönelik kurulum anlatımı için `README.md`, native motor tasarımı için `docs/NATIVE_ENGINE.md`, imzalama ayrıntıları için `docs/SIGNING.md` kullanılmalıdır.

## 1. Projenin amacı

VPNToris, macOS menu bar üzerinde çalışan ve aynı anda birden fazla kurumsal VPN bağlantısını yönetmek için geliştirilmiş bir SwiftUI + Go uygulamasıdır.

Temel problem şudur: Bir VPN’e bağlanıldığında diğer özel ağların veya normal internet bağlantısının bozulmaması. Her profil yalnızca kendisine ait CIDR hedeflerine route sağlar. Örneğin:

```text
VPN A → 10.38.0.0/16
VPN B → 10.68.236.0/24
Normal internet → macOS mevcut default route
```

Şifreler ve PSK’ler profil JSON’una yazılmaz; macOS Keychain kullanılır. OTP, bağlantı başladıktan ve gateway challenge gönderdikten sonra UI üzerinden alınır.

## 2. Mevcut durum ve kapsam

- Mevcut release: `1.2.0`
- Hedef macOS: 13+, Apple Silicon ve Intel
- UI: native SwiftUI menu bar uygulaması
- Controller: Go HTTP API, yalnızca `127.0.0.1:17984`
- Privileged işlemler: macOS LaunchDaemon/native helper
- Desteklenen profil tipleri: FortiGate SSL, FortiClient uyumlu IPsec, OpenConnect protokolleri, OpenVPN
- FortiGate SSL ve FortiClient IPsec gerçek ortamda denenmiştir.
- GlobalProtect/OpenConnect ve OpenVPN entegrasyonu kodda vardır ancak her gateway varyantı production ortamında end-to-end doğrulanmış değildir.
- IPsec DH Group 20 (`ECP-384`) desteklenir.
- IPsec IKEv1/IKEv2, Main/Aggressive, XAuth/EAP, Mode Config, NAT-T, DPD, Phase 1/2, PFS ve proposal seçenekleri bulunur.
- v1.x macOS akışında Docker yolu hâlâ repository’de bulunur; native engine paketleri de macOS installer’a gömülür.
- Docker’sız Linux/Windows native engine, v2.0 kapsamıdır; v1.x’e eklenmemelidir.

## 3. Mimari

```text
SwiftUI menu bar app
        │ HTTP localhost API
        ▼
Go controller (profiles, sessions, routes, traffic, OTP, reconnect)
        ├── native macOS helper → bundled VPN engines
        ├── Docker Engine → legacy/container VPN path
        └── route helper / tun2socks → profile CIDR routes
```

### SwiftUI katmanı

Ana dosya `vpntoris-tray/swift/VPNTorisApp.swift` içindedir. Aynı dosyada:

- Profile modelleri ve editör
- Menu bar popover
- Connect/disconnect/cancel/OTP/reset işlemleri
- Traffic counters ve sparkline
- Active Connections, routes, logs, diagnostics, history ve help ekranları
- Import/discovery ve backup/restore UI
- Touch ID/sudo yardım ekranı

Popover `NSPopover` ile açılır. `popover.behavior = .transient` ve sabit `contentSize` davranışını bozacak mouse-tracking, hover veya dinamik pencere konumu eklenmemelidir.

### Go controller

`vpntoris-tray/main.go` HTTP API ve profil/session state’in merkezidir. Yeni endpoint eklerken:

1. HTTP method kontrolü yap.
2. Input validation uygula.
3. Secret’ları loglama.
4. Native ve Docker yollarını birbirinden ayır.
5. Route durumunu `waiting`, `adding`, `ready`, `failed` olarak güncelle.

Başlıca endpoint’ler profil, connect/disconnect, routes, traffic, flows, logs, diagnostics, recover ve reset işlemleridir. Reset endpoint’i tüm bağlantıları kapatır, route’ları temizler, OTP/reconnect state’ini sıfırlar ve profilleri silmez.

### Native engine

Native ortak altyapı `vpntoris-tray/internal/nativeengine` altındadır:

- `types.go`: engine/session/manifest tipleri
- `manifest.go`: engine manifest ve capability doğrulama
- `plan.go`: platform işlem planı
- `journal.go`: geri alınabilir state journal’ı
- `manager.go`: lifecycle yönetimi
- `supervisor.go`: engine process supervision
- `process_unix.go`, `process_windows.go`: platform process ayrımı

macOS protocol adapter’ları root seviyesindeki şu dosyalardadır:

- `native_forti_darwin.go`: FortiGate SSL/native Fortinet
- `native_ipsec_darwin.go`: strongSwan/charon/VICI IPsec
- `native_openconnect_darwin.go`: OpenConnect ve gateway protocol’leri
- `native_openvpn_darwin.go`: OpenVPN management/auth flow

`*_other.go` dosyaları Linux/Windows için geçici stub’dır. Bunları v2.0 platform işi başlamadan genişletme.

### Native macOS helper

`vpntoris-tray/cmd/vpntoris-native-helper/main_darwin.go` privileged servis mantığını taşır. Helper:

- Session başlatır/durdurur
- Route ve interface sahipliğini profile göre izler
- IPsec charon ve VICI socket yönetir
- OTP için profile-specific FIFO kullanır
- `reset` aksiyonunda tüm session’ları kapatır

Native helper güncellenince eski charon process’i bellekte eski plugin’i tutabilir. `scripts/macos/native-postinstall` bu nedenle eski VPNToris charon process’ini sonlandırıp VICI socket/pid dosyalarını temizler.

## 4. IPsec ve OTP akışı

FortiGate XAuth akışında OTP bağlantı başlarken değil, gateway challenge gönderdiğinde gelir. Doğru sıra:

1. UI connect isteği gönderir.
2. strongSwan IKE negotiation yapar.
3. Gateway `X_TYPE/X_USER/X_PWD` veya `X_TYPE/X_CODE/X_MSG` challenge gönderir.
4. Plugin FIFO üzerinden UI’dan OTP bekler.
5. UI OTP girer; native helper FIFO’ya yazar.
6. Plugin `XAUTH_PASSCODE` olarak cevap verir.
7. CHILD_SA kurulunca route aşaması başlar.

OTP’nin erken istenmesi veya her yeniden bağlanmada popup açılması hatalı davranıştır. `xauth-generic-otp.patch` içinde `XAUTH_PASSCODE` desteği korunmalıdır.

VICI socket varsayılan runtime yolu `/var/run/vpntoris-native/charon.vici`’dir. `Connection refused` görülürse önce charon process’i, sonra socket readiness ve en son `swanctl --load-all` çağrısı kontrol edilir.

## 5. Route ve bağlantı güvenliği

- Sadece kullanıcı tarafından profile eklenen CIDR’lar route edilir.
- Default route değişikliğini varsayılan olarak destekleme.
- Route işlemleri privileged helper üzerinden yapılır; UI’dan doğrudan `osascript` veya shell password prompt ekleme.
- Her session kendi interface/route sahipliğini taşır; başka profilin route’unu silme.
- Route uygulaması VPN healthy olduktan yaklaşık 3 saniye sonra başlar.
- Disconnect, failure ve Reset All Connections yalnızca VPNToris’in sahip olduğu route’ları temizlemelidir.
- Kullanıcının local LAN bloğu ile VPN CIDR’ı çakışabilir; en spesifik route mantığı ve açık hata mesajı korunmalıdır.

## 6. Docker yolu

Docker dosyaları `docker/` altındadır:

- `Dockerfile`: openvpn, openconnect, openfortivpn, strongSwan, danted, dnsmasq ve ağ araçları
- `entrypoint.sh`: seçilen protokole göre container başlangıcı
- `healthcheck.sh`: container sağlık kontrolü
- `xauth-generic-otp.patch`: container strongSwan OTP patch’i
- `danted.conf.template`: SOCKS proxy yapılandırması

Docker Desktop kapalı/kurulu değilse UI uyarı gösterir. Docker image build hatalarında credential helper veya registry erişimi gibi host problemlerini VPN kodu sanma.

## 7. Dosya haritası

| Yol | Sorumluluk |
|---|---|
| `vpntoris-tray/main.go` | localhost API, profil/session/controller |
| `vpntoris-tray/swift/VPNTorisApp.swift` | SwiftUI tray ve tüm macOS UI |
| `vpntoris-tray/cli/main.go` | `vpntorisctl` CLI |
| `vpntoris-tray/internal/nativeengine/` | native engine lifecycle/journal/manifest |
| `vpntoris-tray/internal/fortihelper/` | helper request protokolü ve validation |
| `vpntoris-tray/internal/openvpnconfig/` | OpenVPN config parse/sanitize |
| `vpntoris-tray/routerhelper/` | route helper ve CIDR validation |
| `vpntoris-tray/cmd/vpntoris-native-helper/` | macOS privileged native service |
| `vpntoris-tray/cmd/vpntoris-browser-open/` | external browser auth broker |
| `scripts/macos/` | native engine build/package scripts |
| `scripts/pkg/` | PKG preinstall/postinstall |
| `scripts/release.sh` | app build/sign/DMG/PKG release |
| `scripts/generate-sbom.sh` | release SBOM text çıktısı |
| `docs/NATIVE_ENGINE.md` | v2 native platform tasarımı |
| `docs/SIGNING.md` | signing/notarization akışı |

## 8. Geliştirme komutları

```bash
# Go testleri
cd vpntoris-tray
go test ./...
cd ..

# Swift syntax/type check
xcrun swiftc -typecheck -parse-as-library \
  -target arm64-apple-macos13.0 \
  vpntoris-tray/swift/VPNTorisApp.swift

# SBOM
VERSION=1.2.0 ./scripts/generate-sbom.sh

# Signed release; notarization atlanır
VERSION=1.2.0 ARCH=universal ./scripts/release.sh --skip-notarization

# Native engine package
./scripts/macos/build-native-test-pkg.sh \
  .build/VPNToris-Native-Engine-2.0.0-signed.pkg

# Complete signed installer
VERSION=1.2.0 ARCH=universal ./scripts/macos/build-complete-pkg.sh
```

Release signing kimlikleri `.env` üzerinden alınır. Kimlikleri, certificate fingerprint’lerini, Keychain içeriğini veya VPN credential’larını commit/log/final response içine koyma.

## 9. Test yaklaşımı

Değişiklik türüne göre minimum test:

- Go controller/helper değişikliği: `go test ./...`
- Swift UI değişikliği: `swiftc -typecheck`
- Route değişikliği: route helper unit testleri ve sahiplik/rollback testi
- OTP/IPsec değişikliği: challenge state, FIFO timeout, VICI readiness ve failed CHILD_SA cleanup testi
- Packaging değişikliği: PKG signature check, architecture check, postinstall smoke test
- UI değişikliği: popover’un sabit kalması, trafik güncellemesinde kart konumunun değişmemesi, reset confirm ve cancel akışları

Gerçek VPN testinde gateway, kullanıcı adı, IP, PSK, OTP veya özel CIDR’ları test dosyalarına hard-code etme. Test isimleri ve fixture’lar anonim olmalıdır.

## 10. Secret ve Git kuralları

- `.env`, Keychain, exported profile, diagnostics ZIP ve runtime logları paylaşılmaz.
- `VPNTORIS_PASSWORD`, `VPNTORIS_PSK`, OTP, private key ve certificate secret’ları CLI argümanı olarak kullanma.
- Profile JSON’a password/PSK yazma.
- Kullanıcının istediği açıkça belirtilmedikçe remote push veya merge yapma.
- Main branch’e doğrudan push korumasını değiştirme.
- Destructive cleanup için geniş `rm -rf` kullanma.
- Commit mesajlarında agent adını/AI bilgisini belirtme.

## 11. Release bilgisi

Mevcut release metadata’sı `1.2.0`’dır. Universal complete installer üretildiğinde SHA-256 ayrıca paylaşılır. SBOM release asset olarak installer yanında tutulmalıdır. Signed package üretimi için signing identity kaynak dosyaya gömülmez; `.env` veya CI secret store kullanılır.

## 12. v2.0 sınırı

v2.0 yeni özellik değil, platform mimarisi sürümüdür:

- Docker runtime bağımlılığını kaldırma
- Ortak Go native engine interface’ini tamamlamak
- Linux: `/dev/net/tun`, rtnetlink, systemd-resolved, DEB/RPM/AppImage
- Windows: Wintun, IP Helper API, Windows service, MSI/MSIX
- Platforma göre credential store ve signing
- Crash recovery, journal rollback ve repair command

v2.0 çalışmasına başlamadan önce macOS 1.2.x davranışını bozma. Yeni platform kodu `*_other.go` stub’larını kontrollü biçimde gerçek backend’lere dönüştürmeli; macOS helper ve SwiftUI akışını gereksiz yere yeniden yazmamalıdır.


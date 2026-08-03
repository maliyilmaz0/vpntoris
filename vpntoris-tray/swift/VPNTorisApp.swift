import SwiftUI
import Security
import UserNotifications
import UniformTypeIdentifiers
import CryptoKit
import Charts

struct VPNProfile: Codable, Identifiable, Hashable {
    var id: String { name }
    var name = ""
    var description = ""
    var type = "openfortivpn"
    var host = ""
    var backupGateways: String?
    var failoverThreshold: Int?
    var port = "443"
    var user = ""
    var password = ""
    var twoFactor: Bool?
    var autoReconnect: Bool?
    var connectOnLaunch: Bool?
    var routes = ""
    var domains: String?
    var dnsServers: String?
    var config = ""
    var ipsec: IPSecSettings?
}

struct IPSecSettings: Codable, Hashable {
    var ikeVersion = 2
    var ikeMode = "main"
    var authMode = "eap"
    var preSharedKey = ""
    var localID = ""
    var remoteID = ""
    var modeConfig = true
    var natTraversal = true
    var forceEncap = false
    var mobike = false
    var fragmentation = "yes"
    var dpdAction = "restart"
    var dpdDelay = 30
    var dpdTimeout = 150
    var ikeLifetime = 28800
    var ikeEncryption = "aes256,aes128,aes256gcm16,aes128gcm16,chacha20poly1305"
    var ikeIntegrity = "sha256,sha384,sha512"
    var ikePRF = "prfsha256,prfsha384,prfsha512"
    var dhGroups = "14,19,20,21,31"
    var childLifetime = 3600
    var childLifetimeKB = 0
    var espEncryption = "aes256,aes128,aes256gcm16,aes128gcm16,chacha20poly1305"
    var espIntegrity = "sha256,sha384,sha512"
    var pfs = false
    var pfsGroups = "14,19,20,21,31"
    var replayWindow = 32
    var localSelectors = ""
    var remoteSelectors = ""
    var phase2Proposals: [IPSecProposal]?
}

struct IPSecProposal: Codable, Hashable { var encryption = "aes256"; var authentication = "sha256" }

struct ProfileStatus: Codable, Identifiable {
    var id: String { name }
    let name: String
    let description: String
    let type: String
    let host: String
    let activeGateway: String
    let gatewayCount: Int
    let routes: String
    let connected: Bool
    let twoFactor: Bool
    let autoReconnect: Bool
    let needsOtp: Bool
    let routeStatus: String
}

struct ActiveRoute: Codable, Identifiable { var id: String { profile + cidr }; let profile: String; let cidr: String; let port: String }

struct DockerStatus: Codable {
    let state: String
    let message: String
}

struct TrafficStatus: Codable, Identifiable {
    var id: String { name }
    let name: String
    let received: UInt64
    let sent: UInt64
    let receiveBps: Double
    let sendBps: Double
    let duration: Int64
}

struct HistoryEntry: Codable, Identifiable { let id: String; let profile: String; let event: String; let time: String; let received: UInt64; let sent: UInt64 }
struct RouteMatch: Codable, Identifiable { var id: String { profile + cidr }; let profile: String; let cidr: String; let prefix: Int; let connected: Bool }
struct RouteCheck: Codable { let target: String; let matches: [RouteMatch]; let conflict: Bool }
struct ActiveFlow: Codable, Identifiable { let id: String; let profile: String; let process: String; let pid: Int; let local: String; let remote: String; let remoteIp: String; let port: Int; let `protocol`: String }
struct AnalyticsTraffic: Codable { let received: UInt64; let sent: UInt64 }
struct AnalyticsProfile: Codable, Identifiable { var id: String { name }; let name: String; let received: UInt64; let sent: UInt64; let reconnects: Int; let hourly: [String: AnalyticsTraffic]; let daily: [String: AnalyticsTraffic]; let destinations: [String: Int]; let processes: [String: Int] }
struct AnalyticsSettings: Codable { let hourlyDays: Int; let dailyDays: Int }

enum AppNotifications {
    static func enabled(_ event: String) -> Bool { let key = "notifications.\(event)"; return UserDefaults.standard.object(forKey: key) == nil || UserDefaults.standard.bool(forKey: key) }
    static func send(_ event: String, title: String, body: String, id: String, sound: Bool = true) {
        guard enabled(event) else { return }
        let defaults = UserDefaults.standard
        let quietEnabled = defaults.bool(forKey: "notifications.quietEnabled")
        let hour = Calendar.current.component(.hour, from: Date())
        let start = defaults.object(forKey: "notifications.quietStart") as? Int ?? 22
        let end = defaults.object(forKey: "notifications.quietEnd") as? Int ?? 8
        let quiet = quietEnabled && (start <= end ? hour >= start && hour < end : hour >= start || hour < end)
        let content = UNMutableNotificationContent()
        content.title = title
        content.body = body
        if sound && !quiet && (defaults.object(forKey: "notifications.sound") == nil || defaults.bool(forKey: "notifications.sound")) { content.sound = .default }
        UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: id, content: content, trigger: nil))
    }
}

struct ReleaseAsset: Codable { let name: String; let browserDownloadUrl: URL; enum CodingKeys: String, CodingKey { case name; case browserDownloadUrl = "browser_download_url" } }
struct GitHubRelease: Codable { let tagName: String; let name: String?; let htmlUrl: URL; let assets: [ReleaseAsset]; enum CodingKeys: String, CodingKey { case tagName = "tag_name"; case name; case htmlUrl = "html_url"; case assets } }

struct BackupEnvelope: Codable { let version: Int; let kdf: String; let iterations: Int; let salt: String; let payload: String }

enum BackupCrypto {
    static func encrypt(_ data: Data, password: String) throws -> Data {
        var salt = Data(count: 16)
        let status = salt.withUnsafeMutableBytes { SecRandomCopyBytes(kSecRandomDefault, 16, $0.baseAddress!) }
        guard status == errSecSuccess else { throw NSError(domain: "VPNToris", code: 9, userInfo: [NSLocalizedDescriptionKey: "Could not generate secure random data."]) }
        let iterations = 200_000
        let key = derive(password: password, salt: salt, iterations: iterations)
        let sealed = try AES.GCM.seal(data, using: key)
        guard let combined = sealed.combined else { throw NSError(domain: "VPNToris", code: 10, userInfo: [NSLocalizedDescriptionKey: "Could not create encrypted backup."]) }
        return try JSONEncoder.pretty.encode(BackupEnvelope(version: 1, kdf: "PBKDF2-HMAC-SHA256", iterations: iterations, salt: salt.base64EncodedString(), payload: combined.base64EncodedString()))
    }
    static func decrypt(_ data: Data, password: String) throws -> Data {
        let envelope = try JSONDecoder().decode(BackupEnvelope.self, from: data)
        guard envelope.version == 1, envelope.kdf == "PBKDF2-HMAC-SHA256", envelope.iterations >= 100_000, envelope.iterations <= 1_000_000, let salt = Data(base64Encoded: envelope.salt), let payload = Data(base64Encoded: envelope.payload) else { throw NSError(domain: "VPNToris", code: 11, userInfo: [NSLocalizedDescriptionKey: "Unsupported or damaged backup file."]) }
        let key = derive(password: password, salt: salt, iterations: envelope.iterations)
        return try AES.GCM.open(AES.GCM.SealedBox(combined: payload), using: key)
    }
    private static func derive(password: String, salt: Data, iterations: Int) -> SymmetricKey {
        let key = SymmetricKey(data: Data(password.utf8))
        var input = salt
        input.append(contentsOf: [0, 0, 0, 1])
        var current = Data(HMAC<SHA256>.authenticationCode(for: input, using: key))
        var result = current
        if iterations > 1 {
            for _ in 1..<iterations {
                current = Data(HMAC<SHA256>.authenticationCode(for: current, using: key))
                for index in result.indices { result[index] ^= current[index] }
            }
        }
        return SymmetricKey(data: result)
    }
}

@MainActor final class UpdateChecker: ObservableObject {
    @Published var release: GitHubRelease?
    @Published var state = "Ready to check"
    @Published var checking = false
    @Published var downloading = false
    @Published var downloadedFile: URL?
    var currentVersion: String { Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "0.0.0" }
    var updateAvailable: Bool { guard let release else { return false }; return version(release.tagName, isNewerThan: currentVersion) }

    func check(silent: Bool = false) async {
        checking = true
        if !silent { state = "Checking GitHub Releases…" }
        defer { checking = false }
        do {
            var request = URLRequest(url: URL(string: "https://api.github.com/repos/maliyilmaz0/vpntoris/releases/latest")!)
            request.setValue("application/vnd.github+json", forHTTPHeaderField: "Accept")
            request.setValue("VPNToris/\(currentVersion)", forHTTPHeaderField: "User-Agent")
            let (data, response) = try await URLSession.shared.data(for: request)
            guard (response as? HTTPURLResponse)?.statusCode == 200 else { throw NSError(domain: "VPNToris", code: 2, userInfo: [NSLocalizedDescriptionKey: "GitHub did not return a release."]) }
            release = try JSONDecoder().decode(GitHubRelease.self, from: data)
            state = updateAvailable ? "Version \(release?.tagName ?? "") is available" : "VPNToris is up to date"
            if updateAvailable, let tag = release?.tagName, UserDefaults.standard.string(forKey: "notifications.lastUpdate") != tag {
                AppNotifications.send("update", title: "VPNToris update available", body: "Version \(tag) is ready to download.", id: "vpntoris-update-\(tag)")
                UserDefaults.standard.set(tag, forKey: "notifications.lastUpdate")
            }
        } catch {
            if !silent { state = error.localizedDescription }
        }
    }

    func download() async {
        guard let release, let disk = release.assets.first(where: { $0.name.hasSuffix(".dmg") }), let checksum = release.assets.first(where: { $0.name == disk.name + ".sha256" || $0.name.hasSuffix(".dmg.sha256") }) else { state = "This release does not include a DMG and SHA-256 checksum."; return }
        downloading = true
        state = "Downloading \(disk.name)…"
        defer { downloading = false }
        do {
            async let diskResult = URLSession.shared.data(from: disk.browserDownloadUrl)
            async let checksumResult = URLSession.shared.data(from: checksum.browserDownloadUrl)
            let ((diskData, diskResponse), (checksumData, checksumResponse)) = try await (diskResult, checksumResult)
            guard (diskResponse as? HTTPURLResponse)?.statusCode == 200, (checksumResponse as? HTTPURLResponse)?.statusCode == 200 else { throw NSError(domain: "VPNToris", code: 3, userInfo: [NSLocalizedDescriptionKey: "Release download failed."]) }
            let expected = String(decoding: checksumData, as: UTF8.self).split(whereSeparator: { $0 == " " || $0 == "\t" || $0 == "\n" }).first.map(String.init)?.lowercased() ?? ""
            let actual = SHA256.hash(data: diskData).map { String(format: "%02x", $0) }.joined()
            guard expected.count == 64, expected == actual else { throw NSError(domain: "VPNToris", code: 4, userInfo: [NSLocalizedDescriptionKey: "SHA-256 verification failed. The file was not saved."]) }
            let directory = FileManager.default.urls(for: .downloadsDirectory, in: .userDomainMask)[0].appending(path: "VPNToris Updates", directoryHint: .isDirectory)
            try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
            let destination = directory.appending(path: disk.name)
            try diskData.write(to: destination, options: .atomic)
            downloadedFile = destination
            state = "Downloaded and SHA-256 verified"
        } catch { state = error.localizedDescription }
    }

    private func version(_ candidate: String, isNewerThan current: String) -> Bool {
        let left = candidate.trimmingCharacters(in: CharacterSet(charactersIn: "vV")).split(separator: ".").map { Int($0.prefix(while: { $0.isNumber })) ?? 0 }
        let right = current.split(separator: ".").map { Int($0.prefix(while: { $0.isNumber })) ?? 0 }
        for index in 0..<max(left.count, right.count) {
            let a = index < left.count ? left[index] : 0
            let b = index < right.count ? right[index] : 0
            if a != b { return a > b }
        }
        return false
    }
}

struct BrandIcon: View {
    let size: CGFloat
    var body: some View {
        if let url = Bundle.main.url(forResource: "VPNTorisLogo", withExtension: "png"), let image = NSImage(contentsOf: url) {
            Image(nsImage: image).resizable().scaledToFit().frame(width: size, height: size)
        } else {
            Image(systemName: "shield.lefthalf.filled").resizable().scaledToFit().frame(width: size, height: size).foregroundStyle(.orange)
        }
    }
}

enum ProfileKeychain {
    static let service = "com.vpntoris.credentials"
    static func read(profile: String, field: String) -> String {
        let query: [String: Any] = [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service, kSecAttrAccount as String: "\(profile).\(field)", kSecReturnData as String: true, kSecMatchLimit as String: kSecMatchLimitOne]
        var result: CFTypeRef?
        guard SecItemCopyMatching(query as CFDictionary, &result) == errSecSuccess, let data = result as? Data else { return "" }
        return String(data: data, encoding: .utf8) ?? ""
    }
    static func write(_ value: String, profile: String, field: String) {
        let base: [String: Any] = [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service, kSecAttrAccount as String: "\(profile).\(field)"]
        if value.isEmpty { SecItemDelete(base as CFDictionary); return }
        let data = Data(value.utf8)
        if SecItemUpdate(base as CFDictionary, [kSecValueData as String: data] as CFDictionary) == errSecItemNotFound {
            var item = base; item[kSecValueData as String] = data; SecItemAdd(item as CFDictionary, nil)
        }
    }
    static func delete(profile: String) {
        write("", profile: profile, field: "password")
        write("", profile: profile, field: "psk")
    }
}

@MainActor final class VPNStore: ObservableObject {
    @Published var profiles: [ProfileStatus] = []
    @Published var error = ""
    @Published var busy: Set<String> = []
    @Published var docker = DockerStatus(state: "checking", message: "Checking Docker…")
    @Published var traffic: [String: TrafficStatus] = [:]
    @Published var trafficHistory: [String: [Double]] = [:]
    private let api = URL(string: "http://127.0.0.1:17984")!
    private var previousConnections: [String: Bool] = [:]
    private var previousGateways: [String: String] = [:]
    private var connectionStateInitialized = false
    private var pendingReconnect: Set<String> = []

    func refresh() async {
        do {
            let (data, _) = try await URLSession.shared.data(from: api.appending(path: "api/profiles"))
            let updated = try JSONDecoder().decode([ProfileStatus].self, from: data)
            if connectionStateInitialized {
                for profile in updated {
                    if let previous = previousConnections[profile.name], previous != profile.connected {
                        if !profile.connected && profile.autoReconnect { pendingReconnect.insert(profile.name); notifyConnection(profile.name, connected: false) }
                        else if profile.connected && pendingReconnect.remove(profile.name) != nil { AppNotifications.send("reconnect", title: "VPN reconnected", body: profile.name, id: "vpntoris-reconnected-\(profile.name)") }
                        else { notifyConnection(profile.name, connected: profile.connected) }
                    }
                    if profile.gatewayCount > 1, let previous = previousGateways[profile.name], previous != profile.activeGateway {
                        notifyGatewayChange(profile.name, gateway: profile.activeGateway)
                    }
                }
            }
            profiles = updated
            previousConnections = Dictionary(uniqueKeysWithValues: updated.map { ($0.name, $0.connected) })
            previousGateways = Dictionary(uniqueKeysWithValues: updated.map { ($0.name, $0.activeGateway) })
            connectionStateInitialized = true
        } catch { self.error = error.localizedDescription }
    }

    private func notifyConnection(_ profile: String, connected: Bool) {
        AppNotifications.send(connected ? "connect" : "disconnect", title: connected ? "VPN connected" : "VPN disconnected", body: profile, id: "vpntoris-state-\(profile)-\(connected)", sound: !connected)
    }

    private func notifyGatewayChange(_ profile: String, gateway: String) {
        AppNotifications.send("gateway", title: "VPN gateway changed", body: "\(profile) will use \(gateway)", id: "vpntoris-gateway-\(profile)-\(gateway)")
    }

    func refreshDocker(retry: Bool = false) async {
        do {
            let previousState = docker.state
            var request = URLRequest(url: api.appending(path: "api/docker"))
            if retry { request.httpMethod = "POST" }
            let (data, _) = try await URLSession.shared.data(for: request)
            docker = try JSONDecoder().decode(DockerStatus.self, from: data)
            if previousState == "ready" && docker.state != "ready" { AppNotifications.send("docker", title: "Docker unavailable", body: docker.message, id: "vpntoris-docker-\(docker.state)") }
        } catch {
            docker = DockerStatus(state: "error", message: error.localizedDescription)
        }
    }

    func refreshTraffic() async {
        do {
            let (data, _) = try await URLSession.shared.data(from: api.appending(path: "api/traffic"))
            let items = try JSONDecoder().decode([TrafficStatus].self, from: data)
            traffic = Dictionary(uniqueKeysWithValues: items.map { ($0.name, $0) })
            for item in items {
                var history = trafficHistory[item.name] ?? []
                history.append(item.receiveBps + item.sendBps)
                trafficHistory[item.name] = Array(history.suffix(30))
            }
        } catch {}
    }

    func action(_ action: String, name: String, otp: String = "") async {
        busy.insert(name)
        defer { busy.remove(name) }
        do {
            var parts = URLComponents(url: api.appending(path: "api/action"), resolvingAgainstBaseURL: false)!
            parts.queryItems = [.init(name: "action", value: action), .init(name: "name", value: name)]
            var request = URLRequest(url: parts.url!)
            request.httpMethod = "POST"
            if !otp.isEmpty { request.setValue(otp, forHTTPHeaderField: "X-VPNToris-OTP") }
            if (action == "connect" || action == "arm"), let profile = storedProfile(named: name) {
                request.setValue(profile.password, forHTTPHeaderField: "X-VPNToris-Password")
                request.setValue(profile.ipsec?.preSharedKey ?? "", forHTTPHeaderField: "X-VPNToris-PSK")
            }
            let (data, response) = try await URLSession.shared.data(for: request)
            guard (response as? HTTPURLResponse)?.statusCode == 204 else {
                throw NSError(domain: "VPNToris", code: 1, userInfo: [NSLocalizedDescriptionKey: String(data: data, encoding: .utf8) ?? "Operation failed"])
            }
            error = ""
            try? await Task.sleep(for: .seconds(1))
            await refresh()
        } catch { self.error = error.localizedDescription }
    }

    func logs(for name: String) async throws -> String {
        var parts = URLComponents(url: api.appending(path: "api/logs"), resolvingAgainstBaseURL: false)!; parts.queryItems = [.init(name: "name", value: name)]
        let (data, response) = try await URLSession.shared.data(from: parts.url!); guard (response as? HTTPURLResponse)?.statusCode == 200 else { throw NSError(domain: "VPNToris", code: 1, userInfo: [NSLocalizedDescriptionKey: String(data: data, encoding: .utf8) ?? "Log unavailable"]) }; return String(data: data, encoding: .utf8) ?? ""
    }
    func routes() async throws -> [ActiveRoute] { let (data, _) = try await URLSession.shared.data(from: api.appending(path: "api/routes")); return try JSONDecoder().decode([ActiveRoute].self, from: data) }

    func storedProfile(named name: String) -> VPNProfile? { loadConfigs().first { $0.name == name } }

    func save(_ profile: VPNProfile, replacing oldName: String?) throws {
        var configs = loadConfigs()
        if let oldName { configs.removeAll { $0.name == oldName } }
        configs.removeAll { $0.name == profile.name }
        ProfileKeychain.write(profile.password, profile: profile.name, field: "password")
        ProfileKeychain.write(profile.ipsec?.preSharedKey ?? "", profile: profile.name, field: "psk")
        if let oldName, oldName != profile.name { ProfileKeychain.delete(profile: oldName) }
        var persisted = profile
        persisted.password = ""
        persisted.ipsec?.preSharedKey = ""
        configs.append(persisted)
        let url = try configURL()
        try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        try JSONEncoder.pretty.encode(configs).write(to: url, options: .atomic)
    }

    private func loadConfigs() -> [VPNProfile] {
        guard let url = try? configURL(), let data = try? Data(contentsOf: url) else { return [] }
        return ((try? JSONDecoder().decode([VPNProfile].self, from: data)) ?? []).map { stored in
            var profile = stored
            if profile.password.isEmpty { profile.password = ProfileKeychain.read(profile: profile.name, field: "password") }
            if profile.ipsec != nil && profile.ipsec?.preSharedKey.isEmpty == true { profile.ipsec?.preSharedKey = ProfileKeychain.read(profile: profile.name, field: "psk") }
            return profile
        }
    }

    func deleteCredentials(named name: String) { ProfileKeychain.delete(profile: name) }

    func backupProfiles(includeSecrets: Bool) -> [VPNProfile] {
        loadConfigs().map { profile in var copy = profile; if !includeSecrets { copy.password = ""; copy.ipsec?.preSharedKey = "" }; return copy }
    }

    func restoreProfiles(_ profiles: [VPNProfile]) throws {
        for profile in profiles { try save(profile, replacing: profile.name) }
        Task { await refresh() }
    }

    func migrateLegacyCredentials() {
        guard let url = try? configURL(), let data = try? Data(contentsOf: url), var configs = try? JSONDecoder().decode([VPNProfile].self, from: data) else { return }
        var changed = false
        for index in configs.indices {
            if !configs[index].password.isEmpty {
                ProfileKeychain.write(configs[index].password, profile: configs[index].name, field: "password")
                configs[index].password = ""
                changed = true
            }
            if configs[index].ipsec?.preSharedKey.isEmpty == false {
                ProfileKeychain.write(configs[index].ipsec?.preSharedKey ?? "", profile: configs[index].name, field: "psk")
                configs[index].ipsec?.preSharedKey = ""
                changed = true
            }
        }
        if changed, let encoded = try? JSONEncoder.pretty.encode(configs) { try? encoded.write(to: url, options: .atomic) }
    }

    func connectLaunchProfiles() async {
        for profile in loadConfigs() {
            let connected = profiles.first(where: { $0.name == profile.name })?.connected == true
            if connected && profile.autoReconnect == true { await action("arm", name: profile.name) }
            else if !connected && profile.connectOnLaunch == true && profile.twoFactor != true { await action("connect", name: profile.name) }
        }
    }

    private func configURL() throws -> URL {
        try FileManager.default.url(for: .applicationSupportDirectory, in: .userDomainMask, appropriateFor: nil, create: true)
            .appending(path: "VPNToris/configs.json")
    }
}

extension JSONEncoder { static var pretty: JSONEncoder { let e = JSONEncoder(); e.outputFormatting = [.prettyPrinted, .sortedKeys]; return e } }

struct ProfileEditor: View {
    @Environment(\.dismiss) private var dismiss
    @State var profile: VPNProfile
    @State private var ipsec: IPSecSettings
    let title: String
    let onSave: (VPNProfile) -> Void
    init(profile: VPNProfile, title: String, onSave: @escaping (VPNProfile) -> Void) {
        _profile = State(initialValue: profile)
        var settings = profile.ipsec ?? IPSecSettings()
        settings.ikeEncryption = settings.ikeEncryption.components(separatedBy: ",").first ?? "aes256"
        settings.ikeIntegrity = settings.ikeIntegrity.components(separatedBy: ",").first ?? "sha256"
        settings.ikePRF = settings.ikePRF.components(separatedBy: ",").first ?? "prfsha256"
        settings.dhGroups = settings.dhGroups.components(separatedBy: ",").first ?? "14"
        if settings.ikeVersion == 1 && settings.authMode == "eap" { settings.authMode = "xauth" }
        if settings.phase2Proposals == nil { settings.phase2Proposals = [IPSecProposal()] }
        _ipsec = State(initialValue: settings)
        self.title = title
        self.onSave = onSave
    }
    var body: some View {
        ScrollView { VStack(alignment: .leading, spacing: 14) {
            Text(title).font(.title2.bold())
            TextField("Profile name", text: $profile.name)
            Picker("VPN type", selection: $profile.type) { Text("FortiGate SSL VPN").tag("openfortivpn"); Text("FortiGate IPsec").tag("ipsec"); Text("GlobalProtect / OpenConnect").tag("openconnect"); Text("OpenVPN").tag("openvpn") }
            HStack { TextField("Host", text: $profile.host); TextField("Port", text: $profile.port).frame(width: 80) }
            TextField("Backup gateways, one per line", text: Binding(get: { profile.backupGateways ?? "" }, set: { profile.backupGateways = $0 }), axis: .vertical).lineLimit(2...4)
            Stepper("Switch gateway after \(profile.failoverThreshold ?? 2) failed reconnect attempts", value: Binding(get: { max(profile.failoverThreshold ?? 2, 1) }, set: { profile.failoverThreshold = $0 }), in: 1...10)
            TextField("Username", text: $profile.user)
            SecureField("Password", text: $profile.password)
            Toggle("Ask for 2FA / OTP when connecting", isOn: Binding(get: { profile.twoFactor ?? false }, set: { profile.twoFactor = $0 }))
            Toggle("Reconnect automatically if the tunnel drops", isOn: Binding(get: { profile.autoReconnect ?? true }, set: { profile.autoReconnect = $0 }))
            Toggle("Connect when VPNToris opens", isOn: Binding(get: { profile.connectOnLaunch ?? false }, set: { profile.connectOnLaunch = $0 })).disabled(profile.twoFactor ?? false)
            TextField("VPN routes (10.68.0.0/16, …)", text: $profile.routes)
            TextField("Split DNS domains (corp.example.com, …)", text: Binding(get: { profile.domains ?? "" }, set: { profile.domains = $0 }))
            TextField("VPN DNS servers (10.0.0.53, …)", text: Binding(get: { profile.dnsServers ?? "" }, set: { profile.dnsServers = $0 }))
            TextField("Description", text: $profile.description)
            if profile.type == "openvpn" { TextEditor(text: $profile.config).font(.system(.caption, design: .monospaced)).frame(height: 100).overlay(RoundedRectangle(cornerRadius: 6).stroke(.quaternary)) }
            if profile.type == "ipsec" { ipsecSettings }
            HStack { Spacer(); Button("Cancel") { dismiss() }; Button("Save") { if profile.type == "ipsec" { profile.ipsec = ipsec }; onSave(profile); dismiss() }.buttonStyle(.borderedProminent).disabled(profile.name.isEmpty || profile.host.isEmpty) }
        }.padding(22) }.frame(width: 520, height: profile.type == "ipsec" ? 680 : 480)
    }

    @ViewBuilder private var ipsecSettings: some View {
        Divider(); Text("IPsec Advanced Settings").font(.headline)
        GroupBox("VPN Settings") { VStack(alignment: .leading, spacing: 10) {
            Picker("IKE", selection: $ipsec.ikeVersion) { Text("Version 1").tag(1); Text("Version 2").tag(2) }.pickerStyle(.segmented).onChange(of: ipsec.ikeVersion) { version in if version == 1 && ipsec.authMode == "eap" { ipsec.authMode = "xauth" } }
            if ipsec.ikeVersion == 1 { Picker("Exchange mode", selection: $ipsec.ikeMode) { Text("Main").tag("main"); Text("Aggressive").tag("aggressive") } }
            Picker("Extended authentication", selection: $ipsec.authMode) { Text("None").tag("none"); Text("XAuth").tag("xauth"); if ipsec.ikeVersion == 2 { Text("EAP").tag("eap") } }
            SecureField("Pre-shared key", text: $ipsec.preSharedKey)
            Toggle("Mode Config / virtual IP", isOn: $ipsec.modeConfig)
            Toggle("NAT Traversal", isOn: $ipsec.natTraversal); Toggle("Force UDP encapsulation", isOn: $ipsec.forceEncap)
            if ipsec.ikeVersion == 2 { Toggle("MOBIKE", isOn: $ipsec.mobike) }
        }.padding(6) }
        DisclosureGroup("Phase 1 (IKE SA)") { VStack(alignment: .leading, spacing: 9) {
            Picker("Encryption", selection: $ipsec.ikeEncryption) { ForEach(encryptionAlgorithms, id: \.self) { Text($0.uppercased()).tag($0) } }
            Picker("Authentication", selection: $ipsec.ikeIntegrity) { ForEach(authenticationAlgorithms, id: \.self) { Text($0.uppercased()).tag($0) } }
            if ipsec.ikeVersion == 2 { Picker("PRF", selection: $ipsec.ikePRF) { ForEach(prfAlgorithms, id: \.self) { Text($0.uppercased()).tag($0) } } }
            Picker("DH Group", selection: $ipsec.dhGroups) { ForEach(dhGroups, id: \.self) { Text("Group \($0)").tag($0) } }
            HStack { TextField("Key life (seconds)", value: $ipsec.ikeLifetime, format: .number); TextField("Local ID", text: $ipsec.localID); TextField("Remote ID", text: $ipsec.remoteID) }
            HStack { Picker("DPD", selection: $ipsec.dpdAction) { Text("Restart").tag("restart"); Text("Clear").tag("clear"); Text("Hold").tag("trap"); Text("None").tag("none") }; TextField("Delay", value: $ipsec.dpdDelay, format: .number); TextField("Timeout", value: $ipsec.dpdTimeout, format: .number) }
            Picker("Fragmentation", selection: $ipsec.fragmentation) { Text("Yes").tag("yes"); Text("Accept").tag("accept"); Text("No").tag("no") }
        }.padding(.top, 8) }
        DisclosureGroup("Phase 2 (CHILD SA / ESP)") { VStack(alignment: .leading, spacing: 9) {
            ForEach(Array((ipsec.phase2Proposals ?? []).indices), id: \.self) { index in
                HStack {
                    Picker("Encryption", selection: Binding(get: { ipsec.phase2Proposals?[index].encryption ?? "aes256" }, set: { ipsec.phase2Proposals?[index].encryption = $0 })) { ForEach(encryptionAlgorithms, id: \.self) { Text($0.uppercased()).tag($0) } }
                    Picker("Authentication", selection: Binding(get: { ipsec.phase2Proposals?[index].authentication ?? "sha256" }, set: { ipsec.phase2Proposals?[index].authentication = $0 })) { ForEach(authenticationAlgorithms, id: \.self) { Text($0.uppercased()).tag($0) } }
                    Button { ipsec.phase2Proposals?.remove(at: index) } label: { Image(systemName: "minus.circle") }.buttonStyle(.borderless).disabled((ipsec.phase2Proposals?.count ?? 0) <= 1)
                }
            }
            Button("Add Proposal", systemImage: "plus") { ipsec.phase2Proposals = (ipsec.phase2Proposals ?? []) + [IPSecProposal()] }
            HStack { Toggle("PFS", isOn: $ipsec.pfs); TextField("PFS DH groups", text: $ipsec.pfsGroups).disabled(!ipsec.pfs) }
            HStack { TextField("Key life seconds", value: $ipsec.childLifetime, format: .number); TextField("Key life KB", value: $ipsec.childLifetimeKB, format: .number); TextField("Replay window", value: $ipsec.replayWindow, format: .number) }
            TextField("Local traffic selectors", text: $ipsec.localSelectors)
            TextField("Remote traffic selectors (defaults to VPN Routes)", text: $ipsec.remoteSelectors)
        }.padding(.top, 8) }
    }

    private func algorithmHint(_ text: String) -> some View { Text(text).font(.caption2).foregroundStyle(.secondary).textSelection(.enabled) }
    private var encryptionAlgorithms: [String] { ["aes128", "aes192", "aes256", "aes128gcm16", "aes256gcm16", "chacha20poly1305", "3des", "des"] }
    private var authenticationAlgorithms: [String] { ["md5", "sha1", "sha256", "sha384", "sha512"] }
    private var prfAlgorithms: [String] { ["prfmd5", "prfsha1", "prfsha256", "prfsha384", "prfsha512"] }
    private var dhGroups: [String] { ["1", "2", "5", "14", "15", "16", "17", "18", "19", "20", "21", "31", "32"] }
}

struct ContentView: View {
    @StateObject private var store = VPNStore()
    @StateObject private var updater = UpdateChecker()
    @State private var editing: VPNProfile?
    @State private var oldName: String?
    @State private var deleting: ProfileStatus?
    @State private var showTouchIDHelp = false
    @State private var diagnostics: DiagnosticsTarget?
    @State private var showHistory = false
    @State private var showRouteTest = false
    @State private var showFlows = false
    @State private var showImporter = false
    @State private var showUpdates = false
    @State private var showAnalytics = false
    @State private var showNotifications = false
    @State private var showBackup = false
    @State private var showLanguage = false
    @State private var importError = ""
    @State private var pendingOTP: Set<String> = []
    @State private var submittedOTP: Set<String> = []
    @State private var otpCodes: [String: String] = [:]

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                BrandIcon(size: 38)
                VStack(alignment: .leading) { Text("VPNToris").font(.title2.bold()); Text("Private routes, isolated tunnels").font(.caption).foregroundStyle(.secondary) }
                Spacer()
                Menu {
                    Button("Active Routes", systemImage: "point.3.connected.trianglepath.dotted") { diagnostics = .routes }
                    Button("Active Connections", systemImage: "point.3.filled.connected.trianglepath.dotted") { showFlows = true }
                    Button("Route Tester", systemImage: "scope") { showRouteTest = true }
                    Button("Connection History", systemImage: "clock.arrow.circlepath") { showHistory = true }
                    Button("Traffic Analytics", systemImage: "chart.xyaxis.line") { showAnalytics = true }
                    Button("Notifications", systemImage: "bell.badge") { showNotifications = true }
                    Button("Language", systemImage: "character.bubble") { showLanguage = true }
                    Divider()
                    Button("Import VPN Profile…", systemImage: "square.and.arrow.down") { showImporter = true }
                    Button("Backup and Restore…", systemImage: "lock.doc") { showBackup = true }
                    Button("Export Diagnostics…", systemImage: "wrench.and.screwdriver") { Task { await exportDiagnostics() } }
                    Button("Check for Updates…", systemImage: "arrow.triangle.2.circlepath") { showUpdates = true; Task { await updater.check() } }
                } label: { Image(systemName: "ellipsis.circle") }.menuStyle(.borderlessButton).frame(width: 34)
                Button { oldName = nil; editing = VPNProfile() } label: { Image(systemName: "plus") }.buttonStyle(.bordered)
            }.padding(18)
            Divider()
            if store.docker.state != "ready" {
                HStack(spacing: 10) {
                    if store.docker.state == "checking" || store.docker.state == "building" { ProgressView().controlSize(.small) }
                    else { Image(systemName: store.docker.state == "missing" ? "shippingbox" : "exclamationmark.triangle.fill").foregroundStyle(.orange) }
                    VStack(alignment: .leading, spacing: 2) {
                        Text(dockerTitle).font(.callout.bold())
                        Text(store.docker.message).font(.caption).foregroundStyle(.secondary).lineLimit(3)
                    }
                    Spacer()
                    if store.docker.state == "stopped" {
                        Button("Open Docker") { NSWorkspace.shared.open(URL(fileURLWithPath: "/Applications/Docker.app")) }.buttonStyle(.borderedProminent)
                    }
                    if store.docker.state == "missing" {
                        Button("Download") { NSWorkspace.shared.open(URL(string: "https://www.docker.com/products/docker-desktop/")!) }.buttonStyle(.borderedProminent)
                    }
                    if store.docker.state == "stopped" || store.docker.state == "error" {
                        Button("Retry") { Task { await store.refreshDocker(retry: true) } }.buttonStyle(.bordered)
                    }
                }.padding(10).background(.orange.opacity(0.12))
            }
            if !store.error.isEmpty { Text(store.error).font(.caption).foregroundStyle(.red).padding(10) }
            List {
                ForEach(store.profiles) { profile in
                        VStack(alignment: .leading, spacing: 10) {
                            HStack {
                                Image(systemName: profileTypeIcon(profile.type)).font(.title3).foregroundStyle(profile.connected ? .green : .secondary).frame(width: 28)
                                VStack(alignment: .leading, spacing: 3) { Text(profile.name).font(.headline); Text("\(profileTypeName(profile.type)) · \(profile.activeGateway)").font(.caption).foregroundStyle(.secondary); if profile.gatewayCount > 1 { Text("Gateway failover · \(profile.gatewayCount) endpoints").font(.caption2).foregroundStyle(.orange) } }
                                Spacer()
                                if store.busy.contains(profile.name) && !profile.connected {
                                    ProgressView().controlSize(.small)
                                    Text("Connecting…").font(.caption).foregroundStyle(.secondary)
                                    Button("Cancel", role: .destructive) {
                                        pendingOTP.remove(profile.name); submittedOTP.remove(profile.name); otpCodes[profile.name] = ""
                                        Task { await store.action("disconnect", name: profile.name) }
                                    }.buttonStyle(.bordered).tint(.red)
                                } else { Button(profile.connected ? "Disconnect" : "Connect") {
                                    if !profile.connected && profile.twoFactor {
                                        pendingOTP.insert(profile.name)
                                        submittedOTP.remove(profile.name)
                                        otpCodes[profile.name] = ""
                                        Task {
                                            await store.action("connect", name: profile.name)
                                            pendingOTP.remove(profile.name)
                                            submittedOTP.remove(profile.name)
                                        }
                                    } else { Task { await store.action(profile.connected ? "disconnect" : "connect", name: profile.name) } }
                                }
                                    .buttonStyle(.borderedProminent).tint(profile.connected ? .red : .green).disabled(!profile.connected && store.docker.state != "ready")
                                }
                            }
                            Label(profile.routes.isEmpty ? "No routes configured" : profile.routes, systemImage: "point.3.connected.trianglepath.dotted").font(.caption).foregroundStyle(.cyan)
                            if profile.routeStatus == "waiting" { Label("VPN connected · routes will be added in 3 seconds", systemImage: "timer").font(.caption).foregroundStyle(.orange) }
                            if profile.routeStatus == "adding" { HStack { ProgressView().controlSize(.small); Text("Adding routes…").font(.caption) }.foregroundStyle(.orange) }
                            if profile.routeStatus == "ready" && profile.connected { Label("Routes active", systemImage: "checkmark.circle.fill").font(.caption).foregroundStyle(.green) }
                            if profile.routeStatus == "failed" { Label("Routes could not be added", systemImage: "exclamationmark.triangle.fill").font(.caption).foregroundStyle(.red) }
                            if profile.connected, let traffic = store.traffic[profile.name] {
                                HStack(spacing: 12) {
                                    Label(byteRate(traffic.receiveBps), systemImage: "arrow.down").foregroundStyle(.green)
                                    Label(byteRate(traffic.sendBps), systemImage: "arrow.up").foregroundStyle(.blue)
                                    Text("↓ \(byteCount(traffic.received))  ↑ \(byteCount(traffic.sent))")
                                    Spacer()
                                    Text(duration(traffic.duration)).foregroundStyle(.secondary)
                                }.font(.caption.monospacedDigit())
                                TrafficSparkline(values: store.trafficHistory[profile.name] ?? []).frame(height: 26)
                            }
                            if pendingOTP.contains(profile.name) && !profile.connected {
                                VStack(alignment: .leading, spacing: 8) {
                                    HStack { ProgressView().controlSize(.small); Text(submittedOTP.contains(profile.name) ? "OTP submitted, connecting…" : "Connecting — waiting for OTP").font(.caption).foregroundStyle(.secondary) }
                                    HStack {
                                        SecureField("2FA / OTP code", text: Binding(get: { otpCodes[profile.name] ?? "" }, set: { otpCodes[profile.name] = $0 })).textFieldStyle(.roundedBorder).onSubmit { submitOTP(profile) }
                                        Button("Submit OTP") { submitOTP(profile) }.buttonStyle(.borderedProminent).disabled((otpCodes[profile.name] ?? "").trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                                        Button("Cancel", role: .destructive) { cancelOTP(profile) }
                                    }
                                }.padding(10).background(.orange.opacity(0.10), in: RoundedRectangle(cornerRadius: 9))
                            }
                            HStack {
                                if profile.connected { Button("Reapply Routes") { Task { await store.action("route", name: profile.name) } } }
                                Button("Logs") { diagnostics = .logs(profile.name) }; Button("Edit") { oldName = profile.name; editing = store.storedProfile(named: profile.name) }
                                Spacer()
                            }.buttonStyle(.borderless)
                        }.padding(14).background(.thinMaterial, in: RoundedRectangle(cornerRadius: 14)).listRowInsets(EdgeInsets(top: 5, leading: 14, bottom: 5, trailing: 14)).listRowSeparator(.hidden).listRowBackground(Color.clear).swipeActions(edge: .trailing, allowsFullSwipe: false) { Button(role: .destructive) { deleting = profile } label: { Image(systemName: "trash.fill") }.tint(.red).accessibilityLabel("Delete") }
                }
            }.listStyle(.plain).scrollContentBackground(.hidden).animation(.easeInOut(duration: 0.25), value: store.profiles.map(\.name))
            Divider(); HStack { Circle().fill(store.docker.state == "ready" ? Color.green : Color.orange).frame(width: 7); Text(store.docker.state == "ready" ? "Docker ready" : "Docker unavailable").font(.caption).foregroundStyle(.secondary); Text("v\(updater.currentVersion)").font(.caption.monospacedDigit()).foregroundStyle(.tertiary).help("VPNToris version \(updater.currentVersion)"); Spacer(); Button("Touch ID") { showTouchIDHelp = true }.buttonStyle(.borderless); Button("Quit") { NSApplication.shared.terminate(nil) }.buttonStyle(.borderless) }.padding(12)
        }.frame(width: 410, height: 560).task {
            store.migrateLegacyCredentials()
            await store.refresh()
            await store.connectLaunchProfiles()
            await store.refreshDocker(retry: true)
            await updater.check(silent: true)
            while store.docker.state == "checking" || store.docker.state == "building" {
                try? await Task.sleep(for: .seconds(2))
                await store.refreshDocker()
            }
            while !Task.isCancelled {
                await store.refreshTraffic()
                await store.refresh()
                for profile in store.profiles where profile.needsOtp && !pendingOTP.contains(profile.name) {
                    pendingOTP.insert(profile.name)
                    otpCodes[profile.name] = ""
                    notifyOTP(profile.name)
                }
                try? await Task.sleep(for: .seconds(1))
            }
        }
        .sheet(item: $editing) { value in ProfileEditor(profile: value, title: oldName == nil ? "Add VPN Profile" : "Edit VPN Profile") { profile in try? store.save(profile, replacing: oldName); Task { await store.refresh() } } }
        .sheet(isPresented: $showTouchIDHelp) { TouchIDHelpView() }
        .sheet(item: $diagnostics) { target in DiagnosticsView(store: store, target: target) }
        .sheet(isPresented: $showHistory) { HistoryView() }
        .sheet(isPresented: $showRouteTest) { RouteTestView() }
        .sheet(isPresented: $showFlows) { ActiveFlowsView() }
        .sheet(isPresented: $showUpdates) { UpdateView(updater: updater) }
        .sheet(isPresented: $showAnalytics) { AnalyticsView() }
        .sheet(isPresented: $showNotifications) { NotificationSettingsView() }
        .sheet(isPresented: $showBackup) { BackupView(store: store) }
        .sheet(isPresented: $showLanguage) { LanguageSettingsView() }
        .fileImporter(isPresented: $showImporter, allowedContentTypes: [.data, .plainText], allowsMultipleSelection: false) { result in
            do { if let url = try result.get().first { oldName = nil; editing = try importedProfile(from: url) } } catch { importError = error.localizedDescription }
        }
        .alert("Import failed", isPresented: Binding(get: { !importError.isEmpty }, set: { if !$0 { importError = "" } })) { Button("OK") { importError = "" } } message: { Text(importError) }
        .alert("Delete \(deleting?.name ?? "profile")?", isPresented: Binding(get: { deleting != nil }, set: { if !$0 { deleting = nil } })) {
            Button("Cancel", role: .cancel) { deleting = nil }
            Button("Delete", role: .destructive) { if let p = deleting { store.deleteCredentials(named: p.name); Task { await store.action("delete", name: p.name) } }; deleting = nil }
        }
    }

    private var dockerTitle: String {
        switch store.docker.state {
        case "missing": return "Docker Desktop is required"
        case "stopped": return "Docker Desktop is not running"
        case "building": return "Preparing VPN engine"
        case "error": return "Docker setup failed"
        default: return "Checking Docker"
        }
    }

    private func profileTypeName(_ type: String) -> String { switch type { case "openfortivpn": return "FortiGate SSL VPN"; case "ipsec": return "FortiGate IPsec"; case "openconnect": return "GlobalProtect / OpenConnect"; case "openvpn": return "OpenVPN"; default: return "VPN" } }
    private func profileTypeIcon(_ type: String) -> String { switch type { case "ipsec": return "lock.shield.fill"; case "openconnect": return "network.badge.shield.half.filled"; case "openvpn": return "point.3.connected.trianglepath.dotted"; default: return "shield.lefthalf.filled" } }

    private func byteRate(_ value: Double) -> String { ByteCountFormatter.string(fromByteCount: Int64(value), countStyle: .file) + "/s" }
    private func byteCount(_ value: UInt64) -> String { ByteCountFormatter.string(fromByteCount: Int64(value), countStyle: .file) }
    private func duration(_ seconds: Int64) -> String {
        let hours = seconds / 3600
        let minutes = (seconds % 3600) / 60
        let remaining = seconds % 60
        return hours > 0 ? String(format: "%02d:%02d:%02d", hours, minutes, remaining) : String(format: "%02d:%02d", minutes, remaining)
    }

    private func notifyOTP(_ profile: String) {
        AppNotifications.send("otp", title: "VPNToris needs an OTP", body: "\(profile) is reconnecting. Open VPNToris and enter the new verification code.", id: "vpntoris-otp-\(profile)")
    }

    private func importedProfile(from url: URL) throws -> VPNProfile {
        let scoped = url.startAccessingSecurityScopedResource()
        defer { if scoped { url.stopAccessingSecurityScopedResource() } }
        let text = try String(contentsOf: url, encoding: .utf8)
        var profile = VPNProfile()
        profile.name = url.deletingPathExtension().lastPathComponent
        if url.pathExtension.lowercased() == "ovpn" || text.contains("client\n") || text.contains("client\r\n") {
            profile.type = "openvpn"
            profile.config = text
            for line in text.components(separatedBy: .newlines) {
                let fields = line.split(whereSeparator: { $0 == " " || $0 == "\t" })
                if fields.first == "remote", fields.count >= 2 { profile.host = String(fields[1]); if fields.count >= 3 { profile.port = String(fields[2]) }; break }
            }
            return profile
        }
        profile.type = text.localizedCaseInsensitiveContains("ipsec") ? "ipsec" : "openfortivpn"
        profile.host = xmlValue(["server", "gateway", "remote_gateway"], in: text)
        profile.user = xmlValue(["username", "user"], in: text)
        let importedName = xmlValue(["name", "connection_name"], in: text)
        if !importedName.isEmpty { profile.name = importedName }
        let importedPort = xmlValue(["port"], in: text)
        if !importedPort.isEmpty { profile.port = importedPort }
        if profile.type == "ipsec" { profile.ipsec = IPSecSettings() }
        guard !profile.host.isEmpty else { throw NSError(domain: "VPNToris", code: 1, userInfo: [NSLocalizedDescriptionKey: "No OpenVPN remote or FortiClient gateway was found in the selected file."]) }
        return profile
    }

    private func xmlValue(_ names: [String], in text: String) -> String {
        for name in names {
            let escaped = NSRegularExpression.escapedPattern(for: name)
            if let expression = try? NSRegularExpression(pattern: "<\(escaped)[^>]*>\\s*([^<]+)\\s*</\(escaped)>", options: [.caseInsensitive]), let match = expression.firstMatch(in: text, range: NSRange(text.startIndex..., in: text)), let range = Range(match.range(at: 1), in: text) { return String(text[range]).trimmingCharacters(in: .whitespacesAndNewlines) }
        }
        return ""
    }

    private func exportDiagnostics() async {
        do {
            let (data, response) = try await URLSession.shared.data(from: URL(string: "http://127.0.0.1:17984/api/diagnostics")!)
            guard (response as? HTTPURLResponse)?.statusCode == 200 else { throw NSError(domain: "VPNToris", code: 5, userInfo: [NSLocalizedDescriptionKey: "The diagnostics service returned an error."]) }
            let panel = NSSavePanel()
            panel.allowedContentTypes = [.zip]
            panel.nameFieldStringValue = "VPNToris-Diagnostics-\(ISO8601DateFormatter().string(from: Date()).replacingOccurrences(of: ":", with: "-" )).zip"
            if panel.runModal() == .OK, let url = panel.url { try data.write(to: url, options: .atomic) }
        } catch { importError = "Diagnostics: \(error.localizedDescription)" }
    }

    private func submitOTP(_ profile: ProfileStatus) {
        let code = (otpCodes[profile.name] ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        guard !code.isEmpty else { return }; submittedOTP.insert(profile.name)
        Task { await store.action("otp", name: profile.name, otp: code) }
    }
    private func cancelOTP(_ profile: ProfileStatus) {
        pendingOTP.remove(profile.name); submittedOTP.remove(profile.name); otpCodes[profile.name] = ""
        Task { await store.action("disconnect", name: profile.name) }
    }
}

struct LanguageSettingsView: View {
    @Environment(\.dismiss) private var dismiss
    @AppStorage("appLanguage") private var language = "system"
    @State private var restartRequired = false
    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack { Text("Language").font(.title2.bold()); Spacer(); Button("Done") { dismiss() } }
            Picker("Application language", selection: $language) { Text("System Default").tag("system"); Text("English").tag("en"); Text("Türkçe").tag("tr") }.pickerStyle(.radioGroup).onChange(of: language) { value in
                if value == "system" { UserDefaults.standard.removeObject(forKey: "AppleLanguages") } else { UserDefaults.standard.set([value], forKey: "AppleLanguages") }
                restartRequired = true
            }
            if restartRequired { HStack { Text("Restart VPNToris to apply the language change.").foregroundStyle(.secondary); Spacer(); Button("Restart Now") { restart() }.buttonStyle(.borderedProminent) } }
        }.padding(22).frame(width: 460)
    }
    private func restart() { let process = Process(); process.executableURL = URL(fileURLWithPath: "/usr/bin/open"); process.arguments = ["-n", Bundle.main.bundlePath]; try? process.run(); NSApplication.shared.terminate(nil) }
}

struct BackupView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var store: VPNStore
    @State private var encrypted = false
    @State private var includeSecrets = false
    @State private var password = ""
    @State private var confirmation = ""
    @State private var importPassword = ""
    @State private var pendingImport: Data?
    @State private var status = ""
    @State private var working = false
    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack { Text("Backup and Restore").font(.title2.bold()); Spacer(); Button("Done") { dismiss() } }
            GroupBox("Export") { VStack(alignment: .leading, spacing: 10) {
                Toggle("Encrypt backup", isOn: $encrypted).onChange(of: encrypted) { value in if !value { includeSecrets = false; password = ""; confirmation = "" } }
                Toggle("Include passwords and pre-shared keys", isOn: $includeSecrets).disabled(!encrypted)
                if encrypted { SecureField("Backup password, at least 8 characters", text: $password); SecureField("Confirm password", text: $confirmation); Text("PBKDF2-HMAC-SHA256 derives the key and AES-256-GCM authenticates and encrypts the backup.").font(.caption).foregroundStyle(.secondary) }
                HStack { Text(encrypted ? "Encrypted .vpntoris backup" : "Secret-free JSON backup").font(.caption).foregroundStyle(.secondary); Spacer(); if working { ProgressView().controlSize(.small) }; Button("Export…") { Task { await exportBackup() } }.buttonStyle(.borderedProminent).disabled(working || encrypted && (password.count < 8 || password != confirmation)) }
            }.frame(maxWidth: .infinity, alignment: .leading) }
            GroupBox("Restore") { VStack(alignment: .leading, spacing: 10) {
                Text("Existing profiles with the same name are replaced. Review imported profiles before connecting.").font(.caption).foregroundStyle(.secondary)
                Button("Choose Backup…") { chooseBackup() }
                if pendingImport != nil { SecureField("Backup password", text: $importPassword); HStack { Button("Cancel") { pendingImport = nil; importPassword = "" }; Button("Decrypt and Restore") { Task { await restoreEncrypted() } }.buttonStyle(.borderedProminent).disabled(working || importPassword.isEmpty) } }
            }.frame(maxWidth: .infinity, alignment: .leading) }
            if !status.isEmpty { Text(status).font(.callout).foregroundStyle(status.hasPrefix("Error") ? .red : .green) }
        }.padding(22).frame(width: 560)
    }
    private func exportBackup() async {
        working = true
        defer { working = false }
        do {
            let profiles = store.backupProfiles(includeSecrets: encrypted && includeSecrets)
            let plain = try JSONEncoder.pretty.encode(profiles)
            let exportPassword = password
            let data = encrypted ? try await Task.detached { try BackupCrypto.encrypt(plain, password: exportPassword) }.value : plain
            let panel = NSSavePanel()
            panel.allowedContentTypes = encrypted ? [.data] : [.json]
            panel.nameFieldStringValue = encrypted ? "VPNToris-Backup.vpntoris" : "VPNToris-Profiles.json"
            if panel.runModal() == .OK, let url = panel.url { try data.write(to: url, options: .atomic); status = "Backup saved successfully." }
        } catch { status = "Error: \(error.localizedDescription)" }
    }
    private func chooseBackup() {
        let panel = NSOpenPanel()
        panel.allowedContentTypes = [.json, .data]
        panel.allowsMultipleSelection = false
        guard panel.runModal() == .OK, let url = panel.url else { return }
        do {
            let data = try Data(contentsOf: url)
            if let profiles = try? JSONDecoder().decode([VPNProfile].self, from: data) { try store.restoreProfiles(profiles); status = "\(profiles.count) profiles restored."; pendingImport = nil }
            else if (try? JSONDecoder().decode(BackupEnvelope.self, from: data)) != nil { pendingImport = data; importPassword = ""; status = "Enter the backup password." }
            else { throw NSError(domain: "VPNToris", code: 12, userInfo: [NSLocalizedDescriptionKey: "Unsupported backup format."]) }
        } catch { status = "Error: \(error.localizedDescription)" }
    }
    private func restoreEncrypted() async {
        guard let pendingImport else { return }
        working = true
        defer { working = false }
        let password = importPassword
        do { let plain = try await Task.detached { try BackupCrypto.decrypt(pendingImport, password: password) }.value; let profiles = try JSONDecoder().decode([VPNProfile].self, from: plain); try store.restoreProfiles(profiles); self.pendingImport = nil; importPassword = ""; status = "\(profiles.count) encrypted profiles restored." } catch { status = "Error: Wrong password or damaged backup." }
    }
}

struct NotificationSettingsView: View {
    @Environment(\.dismiss) private var dismiss
    @AppStorage("notifications.connect") private var connect = true
    @AppStorage("notifications.disconnect") private var disconnect = true
    @AppStorage("notifications.reconnect") private var reconnect = true
    @AppStorage("notifications.gateway") private var gateway = true
    @AppStorage("notifications.otp") private var otp = true
    @AppStorage("notifications.docker") private var docker = true
    @AppStorage("notifications.routeConflict") private var routeConflict = true
    @AppStorage("notifications.update") private var update = true
    @AppStorage("notifications.sound") private var sound = true
    @AppStorage("notifications.quietEnabled") private var quietEnabled = false
    @AppStorage("notifications.quietStart") private var quietStart = 22
    @AppStorage("notifications.quietEnd") private var quietEnd = 8
    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack { Text("Notifications").font(.title2.bold()); Spacer(); Button("System Settings") { if let url = URL(string: "x-apple.systempreferences:com.apple.Notifications-Settings.extension") { NSWorkspace.shared.open(url) } }; Button("Done") { dismiss() } }
            GroupBox("VPN Events") { VStack(alignment: .leading, spacing: 9) { Toggle("Connected", isOn: $connect); Toggle("Disconnected", isOn: $disconnect); Toggle("Reconnected", isOn: $reconnect); Toggle("Gateway failover", isOn: $gateway); Toggle("OTP required", isOn: $otp) }.frame(maxWidth: .infinity, alignment: .leading) }
            GroupBox("System Events") { VStack(alignment: .leading, spacing: 9) { Toggle("Docker failure", isOn: $docker); Toggle("Route conflict", isOn: $routeConflict); Toggle("Update available", isOn: $update) }.frame(maxWidth: .infinity, alignment: .leading) }
            Toggle("Notification sounds", isOn: $sound)
            GroupBox("Quiet Hours") { VStack(alignment: .leading, spacing: 8) { Toggle("Mute sounds during quiet hours", isOn: $quietEnabled); HStack { Stepper("Start: \(quietStart):00", value: $quietStart, in: 0...23); Stepper("End: \(quietEnd):00", value: $quietEnd, in: 0...23) }.disabled(!quietEnabled); Text("Notifications remain visible; only their sound is muted.").font(.caption).foregroundStyle(.secondary) }.frame(maxWidth: .infinity, alignment: .leading) }
        }.padding(22).frame(width: 520)
    }
}

struct AnalyticsView: View {
    struct Point: Identifiable { let id: String; let label: String; let received: UInt64; let sent: UInt64 }
    @Environment(\.dismiss) private var dismiss
    @State private var profiles: [AnalyticsProfile] = []
    @State private var selection = ""
    @State private var period = "Daily"
    @State private var hourlyDays = 7
    @State private var dailyDays = 90
    var body: some View {
        VStack(spacing: 14) {
            HStack { Text("Traffic Analytics").font(.title2.bold()); Spacer(); Button("Clear", role: .destructive) { Task { await clear() } }; Button("Done") { dismiss() } }
            HStack { Stepper("Hourly: \(hourlyDays) days", value: $hourlyDays, in: 1...30); Stepper("Daily: \(dailyDays) days", value: $dailyDays, in: 7...365); Button("Save Retention") { Task { await saveRetention() } } }.font(.caption)
            if profiles.isEmpty { VStack(spacing: 10) { Image(systemName: "chart.xyaxis.line").font(.largeTitle); Text("No traffic history yet").font(.headline); Text("Totals appear after a VPN transfers data.").foregroundStyle(.secondary) }.frame(maxWidth: .infinity, maxHeight: .infinity) }
            else if let profile = selectedProfile {
                HStack { Picker("Profile", selection: $selection) { ForEach(profiles) { Text($0.name).tag($0.name) } }.frame(maxWidth: 280); Picker("Period", selection: $period) { Text("Hourly").tag("Hourly"); Text("Daily").tag("Daily") }.pickerStyle(.segmented).frame(width: 180); Spacer() }
                HStack(spacing: 12) {
                    metric("Downloaded", bytes(profile.received), "arrow.down", .green)
                    metric("Uploaded", bytes(profile.sent), "arrow.up", .blue)
                    metric("Reconnects", "\(profile.reconnects)", "arrow.clockwise", .orange)
                }
                Chart(points) { point in
                    BarMark(x: .value("Time", point.label), y: .value("Downloaded", point.received)).foregroundStyle(.green)
                    BarMark(x: .value("Time", point.label), y: .value("Uploaded", point.sent)).foregroundStyle(.blue)
                }.chartYAxis { AxisMarks(position: .leading) { value in AxisGridLine(); AxisValueLabel { if let amount = value.as(UInt64.self) { Text(bytes(amount)) } } } }.frame(height: 190)
                HStack(alignment: .top, spacing: 14) {
                    ranking("Top Destinations", values: profile.destinations, icon: "server.rack")
                    ranking("Top Processes", values: profile.processes, icon: "app.connected.to.app.below.fill")
                }
            }
        }.padding(20).frame(width: 760, height: 600).task { await load(); await loadRetention() }
    }
    private var selectedProfile: AnalyticsProfile? { profiles.first { $0.name == selection } ?? profiles.first }
    private var points: [Point] {
        guard let profile = selectedProfile else { return [] }
        let values = period == "Hourly" ? profile.hourly : profile.daily
        return values.keys.sorted().suffix(period == "Hourly" ? 24 : 30).map { key in Point(id: key, label: period == "Hourly" ? String(key.dropFirst(5).prefix(8)).replacingOccurrences(of: "T", with: " ") : String(key.dropFirst(5)), received: values[key]?.received ?? 0, sent: values[key]?.sent ?? 0) }
    }
    private func metric(_ title: String, _ value: String, _ icon: String, _ color: Color) -> some View { VStack(alignment: .leading, spacing: 6) { Label(title, systemImage: icon).foregroundStyle(color); Text(value).font(.title3.bold().monospacedDigit()) }.padding(12).frame(maxWidth: .infinity, alignment: .leading).background(.quaternary, in: RoundedRectangle(cornerRadius: 10)) }
    private func ranking(_ title: String, values: [String: Int], icon: String) -> some View { VStack(alignment: .leading, spacing: 8) { Label(title, systemImage: icon).font(.headline); ForEach(Array(values.sorted { $0.value > $1.value }.prefix(6)), id: \.key) { item in HStack { Text(item.key).lineLimit(1); Spacer(); Text("\(item.value)").monospacedDigit().foregroundStyle(.secondary) }.font(.caption) }; if values.isEmpty { Text("No samples").font(.caption).foregroundStyle(.secondary) } }.padding(12).frame(maxWidth: .infinity, alignment: .leading).background(.quaternary, in: RoundedRectangle(cornerRadius: 10)) }
    private func bytes(_ value: UInt64) -> String { ByteCountFormatter.string(fromByteCount: Int64(value), countStyle: .file) }
    private func load() async { if let url = URL(string: "http://127.0.0.1:17984/api/analytics"), let (data, _) = try? await URLSession.shared.data(from: url), let decoded = try? JSONDecoder().decode([AnalyticsProfile].self, from: data) { profiles = decoded; if selection.isEmpty { selection = decoded.first?.name ?? "" } } }
    private func clear() async { var request = URLRequest(url: URL(string: "http://127.0.0.1:17984/api/analytics")!); request.httpMethod = "DELETE"; _ = try? await URLSession.shared.data(for: request); profiles = []; selection = "" }
    private func loadRetention() async { if let url = URL(string: "http://127.0.0.1:17984/api/analytics-settings"), let (data, _) = try? await URLSession.shared.data(from: url), let settings = try? JSONDecoder().decode(AnalyticsSettings.self, from: data) { hourlyDays = settings.hourlyDays; dailyDays = settings.dailyDays } }
    private func saveRetention() async { var parts = URLComponents(string: "http://127.0.0.1:17984/api/analytics-settings")!; parts.queryItems = [.init(name: "hourlyDays", value: String(hourlyDays)), .init(name: "dailyDays", value: String(dailyDays))]; var request = URLRequest(url: parts.url!); request.httpMethod = "POST"; _ = try? await URLSession.shared.data(for: request) }
}

struct UpdateView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var updater: UpdateChecker
    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack { BrandIcon(size: 48); VStack(alignment: .leading) { Text("Software Update").font(.title2.bold()); Text("Installed: \(updater.currentVersion)").foregroundStyle(.secondary) }; Spacer(); Button("Done") { dismiss() } }
            Divider()
            HStack(spacing: 10) {
                if updater.checking || updater.downloading { ProgressView().controlSize(.small) }
                Image(systemName: updater.updateAvailable ? "arrow.down.circle.fill" : "checkmark.shield.fill").foregroundStyle(updater.updateAvailable ? .blue : .green)
                Text(updater.state)
            }
            if let release = updater.release {
                HStack { Text(release.name ?? release.tagName).font(.headline); Spacer(); Link("Release Notes", destination: release.htmlUrl) }
            }
            Text("Downloads are accepted only when the DMG matches the SHA-256 checksum published with the GitHub release. VPNToris saves the verified installer to Downloads and never replaces the running app automatically.").font(.caption).foregroundStyle(.secondary)
            HStack {
                Button("Check Again") { Task { await updater.check() } }.disabled(updater.checking || updater.downloading)
                Spacer()
                if let file = updater.downloadedFile { Button("Show in Finder") { NSWorkspace.shared.activateFileViewerSelecting([file]) }.buttonStyle(.borderedProminent) }
                else if updater.updateAvailable { Button("Download and Verify") { Task { await updater.download() } }.buttonStyle(.borderedProminent).disabled(updater.downloading) }
            }
        }.padding(22).frame(width: 540)
    }
}

struct ActiveFlowsView: View {
    @Environment(\.dismiss) private var dismiss
    @State private var flows: [ActiveFlow] = []
    var body: some View {
        VStack(spacing: 12) {
            HStack { Text("Active VPN Connections").font(.title2.bold()); Text("Process → destination").font(.caption).foregroundStyle(.secondary); Spacer(); Button("Done") { dismiss() } }
            if flows.isEmpty { VStack(spacing: 10) { Image(systemName: "network.slash").font(.largeTitle); Text("No active VPN flows").font(.headline); Text("Start an SSH, browser or database connection through a configured route.").foregroundStyle(.secondary) }.frame(maxWidth: .infinity, maxHeight: .infinity) }
            else { List(flows) { flow in HStack(spacing: 12) { Image(systemName: icon(flow.process)).frame(width: 24).foregroundStyle(.cyan); VStack(alignment: .leading, spacing: 3) { Text(flow.process).font(.headline); Text("PID \(flow.pid) · \(flow.protocol)").font(.caption).foregroundStyle(.secondary) }; Spacer(); VStack(alignment: .trailing, spacing: 3) { Text(flow.remote).font(.system(.body, design: .monospaced)); Text(flow.profile).font(.caption).foregroundStyle(.orange) } } } }
        }.padding(18).frame(width: 680, height: 460).task { while !Task.isCancelled { await load(); try? await Task.sleep(for: .seconds(2)) } }
    }
    private func load() async { if let url = URL(string: "http://127.0.0.1:17984/api/flows"), let (data, _) = try? await URLSession.shared.data(from: url) { flows = (try? JSONDecoder().decode([ActiveFlow].self, from: data)) ?? [] } }
    private func icon(_ process: String) -> String { let value = process.lowercased(); if value.contains("ssh") { return "terminal" }; if value.contains("code") { return "chevron.left.forwardslash.chevron.right" }; if value.contains("browser") || value.contains("safari") { return "globe" }; return "app.connected.to.app.below.fill" }
}

struct HistoryView: View {
    @Environment(\.dismiss) private var dismiss
    @State private var entries: [HistoryEntry] = []
    var body: some View {
        VStack(spacing: 12) {
            HStack { Text("Connection History").font(.title2.bold()); Spacer(); Button("Clear", role: .destructive) { Task { await clear() } }; Button("Done") { dismiss() } }
            List(entries) { entry in
                HStack { Image(systemName: entry.event == "connected" ? "link.circle.fill" : "xmark.circle").foregroundStyle(entry.event == "connected" ? .green : .secondary); VStack(alignment: .leading) { Text(entry.profile).font(.headline); Text(entry.event.capitalized + " · " + entry.time).font(.caption).foregroundStyle(.secondary) }; Spacer(); Text("↓ \(ByteCountFormatter.string(fromByteCount: Int64(entry.received), countStyle: .file))  ↑ \(ByteCountFormatter.string(fromByteCount: Int64(entry.sent), countStyle: .file))").font(.caption.monospacedDigit()) }
            }
        }.padding(18).frame(width: 620, height: 460).task { await load() }
    }
    private func load() async { if let url = URL(string: "http://127.0.0.1:17984/api/history"), let (data, _) = try? await URLSession.shared.data(from: url) { entries = (try? JSONDecoder().decode([HistoryEntry].self, from: data)) ?? [] } }
    private func clear() async { var request = URLRequest(url: URL(string: "http://127.0.0.1:17984/api/history")!); request.httpMethod = "DELETE"; _ = try? await URLSession.shared.data(for: request); await load() }
}

struct RouteTestView: View {
    @Environment(\.dismiss) private var dismiss
    @State private var target = ""
    @State private var result: RouteCheck?
    @State private var error = ""
    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack { Text("Route Tester").font(.title2.bold()); Spacer(); Button("Done") { dismiss() } }
            HStack { TextField("Destination IPv4 address", text: $target).onSubmit { Task { await check() } }; Button("Check") { Task { await check() } }.buttonStyle(.borderedProminent) }
            if !error.isEmpty { Text(error).foregroundStyle(.red) }
            if let result {
                if result.matches.isEmpty { VStack(spacing: 10) { Image(systemName: "questionmark.circle").font(.largeTitle); Text("No matching VPN route").font(.headline); Text(result.target + " uses the normal system route.").foregroundStyle(.secondary) }.frame(maxWidth: .infinity, maxHeight: .infinity) }
                else { List(result.matches) { match in HStack { Image(systemName: match.connected ? "checkmark.circle.fill" : "circle").foregroundStyle(match.connected ? .green : .secondary); VStack(alignment: .leading) { Text(match.profile); Text(match.cidr).font(.caption.monospaced()).foregroundStyle(.secondary) }; Spacer(); Text("/\(match.prefix)") } }.overlay(alignment: .bottom) { if result.conflict { Label("Conflicting routes have the same prefix", systemImage: "exclamationmark.triangle.fill").foregroundStyle(.orange).padding() } } }
            }
        }.padding(18).frame(width: 540, height: 400)
    }
    private func check() async { do { var parts = URLComponents(string: "http://127.0.0.1:17984/api/route-check")!; parts.queryItems = [.init(name: "target", value: target)]; let (data, response) = try await URLSession.shared.data(from: parts.url!); guard (response as? HTTPURLResponse)?.statusCode == 200 else { throw NSError(domain: "VPNToris", code: 1, userInfo: [NSLocalizedDescriptionKey: String(data: data, encoding: .utf8) ?? "Route check failed"]) }; let decoded = try JSONDecoder().decode(RouteCheck.self, from: data); result = decoded; if decoded.conflict { AppNotifications.send("routeConflict", title: "VPN route conflict", body: "\(decoded.target) matches routes with the same prefix.", id: "vpntoris-route-conflict-\(decoded.target)") }; error = "" } catch { self.error = error.localizedDescription; result = nil } }
}

struct TrafficSparkline: View {
    let values: [Double]
    var body: some View {
        GeometryReader { geometry in
            Canvas { context, size in
                guard values.count > 1, let maximum = values.max(), maximum > 0 else { return }
                var path = Path()
                for (index, value) in values.enumerated() {
                    let x = size.width * CGFloat(index) / CGFloat(values.count - 1)
                    let y = size.height - size.height * CGFloat(value / maximum)
                    if index == 0 { path.move(to: CGPoint(x: x, y: y)) } else { path.addLine(to: CGPoint(x: x, y: y)) }
                }
                context.stroke(path, with: .linearGradient(Gradient(colors: [.cyan, .purple]), startPoint: .zero, endPoint: CGPoint(x: size.width, y: 0)), lineWidth: 2)
            }
        }
    }
}

enum DiagnosticsTarget: Identifiable { case routes, logs(String); var id: String { switch self { case .routes: return "routes"; case .logs(let name): return "logs-" + name } } }

struct DiagnosticsView: View {
    @Environment(\.dismiss) private var dismiss; @ObservedObject var store: VPNStore; let target: DiagnosticsTarget
    @State private var logs = "Loading…"; @State private var routes: [ActiveRoute] = []
    var body: some View { VStack(alignment: .leading, spacing: 12) { HStack { Text(title).font(.title2.bold()); Spacer(); Button("Refresh") { Task { await load() } }; Button("Done") { dismiss() } }; if case .routes = target { List(routes) { route in HStack { VStack(alignment: .leading) { Text(route.cidr).font(.system(.body, design: .monospaced)); Text(route.profile).foregroundStyle(.secondary) }; Spacer(); Text("SOCKS :\(route.port)").font(.caption) } } } else { ScrollView { Text(logs).font(.system(.caption, design: .monospaced)).textSelection(.enabled).frame(maxWidth: .infinity, alignment: .leading) } }; }.padding(18).frame(width: 640, height: 480).task { await load() } }
    private var title: String { switch target { case .routes: return "Active Routes"; case .logs(let name): return name + " Logs" } }
    private func load() async { do { switch target { case .routes: routes = try await store.routes(); case .logs(let name): logs = try await store.logs(for: name) } } catch { logs = error.localizedDescription } }
}

struct TouchIDHelpView: View {
    @Environment(\.dismiss) private var dismiss
    private let command = "sudo sh -c 'touch /etc/pam.d/sudo_local; grep -q \"^[[:space:]]*auth[[:space:]].*pam_tid\\.so\" /etc/pam.d/sudo_local || printf \"auth       sufficient     pam_tid.so\\n\" >> /etc/pam.d/sudo_local' && sudo -k && sudo true"
    private var enabled: Bool {
        guard let text = try? String(contentsOfFile: "/etc/pam.d/sudo_local", encoding: .utf8) else { return false }
        return text.split(separator: "\n").contains { !$0.trimmingCharacters(in: .whitespaces).hasPrefix("#") && $0.contains("pam_tid.so") }
    }
    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack { Image(systemName: "touchid").font(.largeTitle).foregroundStyle(.blue); VStack(alignment: .leading) { Text("Touch ID for sudo").font(.title2.bold()); Text(enabled ? "Enabled on this Mac" : "Not enabled").foregroundStyle(enabled ? .green : .orange) } }
            Text("This enables Touch ID for sudo commands in Terminal. It preserves existing PAM settings and uses sudo_local, which survives macOS updates.").font(.callout).foregroundStyle(.secondary)
            Text("Ready command").font(.headline)
            Text(command).font(.system(.caption, design: .monospaced)).textSelection(.enabled).padding(10).background(.quaternary, in: RoundedRectangle(cornerRadius: 8))
            Text("The macOS administrator dialog used by AppleScript is separate from sudo and may still request your password. VPNToris will use a one-time privileged helper to remove repeated prompts.").font(.caption).foregroundStyle(.secondary)
            HStack { Button("Open Terminal") { NSWorkspace.shared.open(URL(fileURLWithPath: "/System/Applications/Utilities/Terminal.app")) }; Spacer(); Button("Copy Command") { NSPasteboard.general.clearContents(); NSPasteboard.general.setString(command, forType: .string) }.buttonStyle(.borderedProminent); Button("Done") { dismiss() } }
        }.padding(22).frame(width: 520)
    }
}

final class AppDelegate: NSObject, NSApplicationDelegate {
    var daemon: Process?
    private var statusItem: NSStatusItem!
    private let popover = NSPopover()
    private var contentController: NSViewController!
    private var trafficTimer: Timer?

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        UNUserNotificationCenter.current().getNotificationSettings { settings in
            if settings.authorizationStatus == .notDetermined { UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge]) { _, _ in } }
        }
        VPNStore().migrateLegacyCredentials()
        let process = Process(); process.executableURL = Bundle.main.bundleURL.appending(path: "Contents/MacOS/vpntorisd"); process.arguments = ["--daemon", String(ProcessInfo.processInfo.processIdentifier)]; try? process.run(); daemon = process


        popover.contentSize = NSSize(width: 410, height: 560)
        popover.behavior = .applicationDefined
        popover.animates = true
        contentController = NSHostingController(rootView: ContentView())
        popover.contentViewController = contentController

        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        if let button = statusItem.button {
            if let url = Bundle.main.url(forResource: "VPNTorisLogo", withExtension: "png"), let image = NSImage(contentsOf: url) {
                image.size = NSSize(width: 18, height: 18)
                image.isTemplate = false
                button.image = image
            } else {
                button.image = NSImage(systemSymbolName: "shield.lefthalf.filled", accessibilityDescription: "VPNToris")
                button.image?.isTemplate = true
            }
            button.toolTip = "VPNToris"
            button.target = self
            button.action = #selector(togglePopover(_:))
        }
        trafficTimer = Timer.scheduledTimer(timeInterval: 2, target: self, selector: #selector(updateMenuTraffic), userInfo: nil, repeats: true)
        NSWorkspace.shared.notificationCenter.addObserver(self, selector: #selector(systemDidWake), name: NSWorkspace.didWakeNotification, object: nil)
    }

    @objc private func updateMenuTraffic() {
        URLSession.shared.dataTask(with: URL(string: "http://127.0.0.1:17984/api/traffic")!) { [weak self] data, _, _ in
            guard let data, let items = try? JSONDecoder().decode([TrafficStatus].self, from: data) else { return }
            let down = items.reduce(0) { $0 + $1.receiveBps }
            let up = items.reduce(0) { $0 + $1.sendBps }
            DispatchQueue.main.async { self?.statusItem.button?.title = items.isEmpty ? "" : " ↓\(self?.compactRate(down) ?? "0") ↑\(self?.compactRate(up) ?? "0")" }
        }.resume()
    }

    private func compactRate(_ value: Double) -> String {
        if value >= 1_000_000 { return String(format: "%.1fM", value / 1_000_000) }
        if value >= 1_000 { return String(format: "%.0fK", value / 1_000) }
        return String(format: "%.0fB", value)
    }

    @objc private func systemDidWake() {
        var request = URLRequest(url: URL(string: "http://127.0.0.1:17984/api/recover")!)
        request.httpMethod = "POST"
        URLSession.shared.dataTask(with: request).resume()
    }

    @objc private func togglePopover(_ sender: Any?) {
        guard let button = statusItem.button else { return }
        if popover.isShown {
            popover.performClose(sender)
        } else {
            popover.contentViewController = contentController
            popover.show(relativeTo: button.bounds, of: button, preferredEdge: .minY)
            popover.contentViewController?.view.window?.makeKey()
            NSApp.activate(ignoringOtherApps: true)
        }
    }

    func applicationWillTerminate(_ notification: Notification) { trafficTimer?.invalidate(); NSWorkspace.shared.notificationCenter.removeObserver(self); daemon?.terminate() }
}

@main struct VPNTorisApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) var delegate
    var body: some Scene {
        Settings { EmptyView() }
    }
}

import SwiftUI
import Security
import UserNotifications

struct VPNProfile: Codable, Identifiable, Hashable {
    var id: String { name }
    var name = ""
    var description = ""
    var type = "openfortivpn"
    var host = ""
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
    let routes: String
    let connected: Bool
    let twoFactor: Bool
    let needsOtp: Bool
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
    private var connectionStateInitialized = false

    func refresh() async {
        do {
            let (data, _) = try await URLSession.shared.data(from: api.appending(path: "api/profiles"))
            let updated = try JSONDecoder().decode([ProfileStatus].self, from: data)
            if connectionStateInitialized {
                for profile in updated {
                    if let previous = previousConnections[profile.name], previous != profile.connected {
                        notifyConnection(profile.name, connected: profile.connected)
                    }
                }
            }
            profiles = updated
            previousConnections = Dictionary(uniqueKeysWithValues: updated.map { ($0.name, $0.connected) })
            connectionStateInitialized = true
        } catch { self.error = error.localizedDescription }
    }

    private func notifyConnection(_ profile: String, connected: Bool) {
        let content = UNMutableNotificationContent()
        content.title = connected ? "VPN connected" : "VPN disconnected"
        content.body = profile
        content.sound = connected ? nil : .default
        UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: "vpntoris-state-\(profile)-\(connected)", content: content, trigger: nil))
    }

    func refreshDocker(retry: Bool = false) async {
        do {
            var request = URLRequest(url: api.appending(path: "api/docker"))
            if retry { request.httpMethod = "POST" }
            let (data, _) = try await URLSession.shared.data(for: request)
            docker = try JSONDecoder().decode(DockerStatus.self, from: data)
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
            if action == "connect", let profile = storedProfile(named: name) {
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
        for profile in loadConfigs() where profile.connectOnLaunch == true && profile.twoFactor != true {
            if profiles.first(where: { $0.name == profile.name })?.connected != true { await action("connect", name: profile.name) }
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
            Picker("VPN type", selection: $profile.type) { Text("FortiVPN SSL").tag("openfortivpn"); Text("FortiClient IPsec").tag("ipsec"); Text("OpenConnect").tag("openconnect"); Text("OpenVPN").tag("openvpn") }
            HStack { TextField("Host", text: $profile.host); TextField("Port", text: $profile.port).frame(width: 80) }
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
    @State private var editing: VPNProfile?
    @State private var oldName: String?
    @State private var deleting: ProfileStatus?
    @State private var showTouchIDHelp = false
    @State private var diagnostics: DiagnosticsTarget?
    @State private var showHistory = false
    @State private var showRouteTest = false
    @State private var pendingOTP: Set<String> = []
    @State private var submittedOTP: Set<String> = []
    @State private var otpCodes: [String: String] = [:]

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                BrandIcon(size: 38)
                VStack(alignment: .leading) { Text("VPNToris").font(.title2.bold()); Text("Private routes, isolated tunnels").font(.caption).foregroundStyle(.secondary) }
                Spacer(); Button("Routes") { diagnostics = .routes }.buttonStyle(.bordered); Button { showHistory = true } label: { Image(systemName: "clock.arrow.circlepath") }.buttonStyle(.bordered); Button { showRouteTest = true } label: { Image(systemName: "scope") }.buttonStyle(.bordered); Button { oldName = nil; editing = VPNProfile() } label: { Image(systemName: "plus") }.buttonStyle(.bordered)
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
            ScrollView {
                LazyVStack(spacing: 10) {
                    ForEach(store.profiles) { profile in
                        VStack(alignment: .leading, spacing: 10) {
                            HStack {
                                VStack(alignment: .leading, spacing: 3) { Text(profile.name).font(.headline); Text("\(profile.type) · \(profile.host)").font(.caption).foregroundStyle(.secondary) }
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
                                if profile.connected { Button("Use Routes") { Task { await store.action("route", name: profile.name) } } }
                                Button("Logs") { diagnostics = .logs(profile.name) }; Button("Edit") { oldName = profile.name; editing = store.storedProfile(named: profile.name) }
                                Spacer(); Button("Delete", role: .destructive) { deleting = profile }
                            }.buttonStyle(.borderless)
                        }.padding(14).background(.thinMaterial, in: RoundedRectangle(cornerRadius: 14))
                    }
                }.padding(14)
            }
            Divider(); HStack { Circle().fill(store.docker.state == "ready" ? Color.green : Color.orange).frame(width: 7); Text(store.docker.state == "ready" ? "Docker ready" : "Docker unavailable").font(.caption).foregroundStyle(.secondary); Spacer(); Button("Touch ID") { showTouchIDHelp = true }.buttonStyle(.borderless); Button("Quit") { NSApplication.shared.terminate(nil) }.buttonStyle(.borderless) }.padding(12)
        }.frame(width: 410, height: 560).task {
            store.migrateLegacyCredentials()
            await store.refresh()
            await store.connectLaunchProfiles()
            await store.refreshDocker(retry: true)
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

    private func byteRate(_ value: Double) -> String { ByteCountFormatter.string(fromByteCount: Int64(value), countStyle: .file) + "/s" }
    private func byteCount(_ value: UInt64) -> String { ByteCountFormatter.string(fromByteCount: Int64(value), countStyle: .file) }
    private func duration(_ seconds: Int64) -> String {
        let hours = seconds / 3600
        let minutes = (seconds % 3600) / 60
        let remaining = seconds % 60
        return hours > 0 ? String(format: "%02d:%02d:%02d", hours, minutes, remaining) : String(format: "%02d:%02d", minutes, remaining)
    }

    private func notifyOTP(_ profile: String) {
        let content = UNMutableNotificationContent()
        content.title = "VPNToris needs an OTP"
        content.body = "\(profile) is reconnecting. Open VPNToris and enter the new verification code."
        content.sound = .default
        UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: "vpntoris-otp-\(profile)", content: content, trigger: nil))
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
    private func check() async { do { var parts = URLComponents(string: "http://127.0.0.1:17984/api/route-check")!; parts.queryItems = [.init(name: "target", value: target)]; let (data, response) = try await URLSession.shared.data(from: parts.url!); guard (response as? HTTPURLResponse)?.statusCode == 200 else { throw NSError(domain: "VPNToris", code: 1, userInfo: [NSLocalizedDescriptionKey: String(data: data, encoding: .utf8) ?? "Route check failed"]) }; result = try JSONDecoder().decode(RouteCheck.self, from: data); error = "" } catch { self.error = error.localizedDescription; result = nil } }
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
        UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge]) { _, _ in }
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

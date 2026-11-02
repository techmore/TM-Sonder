import SwiftUI
import SonderAPI

/// Account sign-in, shown when the server answers 401.
struct ServerSignInView: View {
    @ObservedObject var model: SonderClientModel
    var onDismiss: () -> Void

    @State private var username = ""
    @State private var password = ""
    @State private var isSigningIn = false
    @State private var errorText: String?
    @FocusState private var focusedField: Field?

    private enum Field { case username, password }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    HStack(spacing: 12) {
                        Image(systemName: "server.rack")
                            .font(.title3.weight(.semibold))
                            .foregroundStyle(SonderPalette.ironGrey)
                            .frame(width: 34, height: 34)
                            .background(SonderPalette.accentMuted, in: RoundedRectangle(cornerRadius: 8))
                        VStack(alignment: .leading, spacing: 2) {
                            Text(model.discovery?.name ?? "TM Sonder")
                                .font(.headline)
                            Text(model.activeBaseURL.absoluteString)
                                .font(.caption)
                                .foregroundStyle(SonderPalette.textLight)
                                .lineLimit(1)
                                .truncationMode(.middle)
                        }
                    }
                    ServerAnswerRow(model: model)
                }

                if model.authStatus?.setupRequired == true {
                    Section {
                        Label("This server has no account yet. Open it in a browser to create the owner account, then sign in here.",
                              systemImage: "person.badge.plus")
                            .font(.callout)
                    }
                }

                Section("Account") {
                    TextField("Username", text: $username)
                        .textContentType(.username)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .focused($focusedField, equals: .username)
                        .submitLabel(.next)
                        .onSubmit { focusedField = .password }
                        .accessibilityIdentifier("signin.username")
                    SecureField("Password", text: $password)
                        .textContentType(.password)
                        .focused($focusedField, equals: .password)
                        .submitLabel(.go)
                        .onSubmit(signIn)
                        .accessibilityIdentifier("signin.password")
                }

                if let errorText {
                    Section {
                        Label(errorText, systemImage: "exclamationmark.triangle")
                            .font(.callout)
                            .foregroundStyle(.red)
                    }
                }

                Section {
                    Button(action: signIn) {
                        HStack {
                            Spacer()
                            if isSigningIn { ProgressView().padding(.trailing, 6) }
                            Text("Sign In").fontWeight(.semibold)
                            Spacer()
                        }
                    }
                    .disabled(isSigningIn || username.trimmingCharacters(in: .whitespaces).isEmpty || password.isEmpty)
                    .accessibilityIdentifier("signin.submit")
                } footer: {
                    Text("Use the same account as the Sonder website. You stay signed in on both the local and public address.")
                }
            }
            .navigationTitle("Sign In")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Not Now", action: onDismiss)
                }
            }
            .onAppear {
                if username.isEmpty, let known = model.signedInUsername { username = known }
                focusedField = username.isEmpty ? .username : .password
            }
        }
        .interactiveDismissDisabled(isSigningIn)
    }

    private func signIn() {
        guard isSigningIn == false, username.isEmpty == false, password.isEmpty == false else { return }
        isSigningIn = true
        errorText = nil
        Task {
            defer { isSigningIn = false }
            do {
                try await model.signIn(username: username, password: password)
                password = ""
            } catch {
                errorText = error.localizedDescription
            }
        }
    }
}

/// Shows whether the server answered the unauthenticated session check, and on which route.
struct ServerAnswerRow: View {
    @ObservedObject var model: SonderClientModel

    var body: some View {
        if model.isSelectingRoute {
            Label("Checking the local address…", systemImage: "antenna.radiowaves.left.and.right")
                .font(.caption)
                .foregroundStyle(SonderPalette.textLight)
        } else if model.authStatus != nil {
            Label("Server answered over the \(model.activeRoute.label.lowercased()). Sign-in required.",
                  systemImage: "checkmark.seal")
                .font(.caption)
                .foregroundStyle(SonderPalette.textLight)
        } else {
            Label("Using the \(model.activeRoute.label.lowercased()) address.", systemImage: routeIcon)
                .font(.caption)
                .foregroundStyle(SonderPalette.textLight)
        }
    }

    private var routeIcon: String {
        model.activeRoute == .local ? "house" : "globe"
    }
}

/// The public + optional local address fields, shared by onboarding and Settings.
struct ServerAddressFields: View {
    @Binding var publicText: String
    @Binding var localText: String
    @Binding var pairingToken: String
    @State private var showsAdvanced = false

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Public address")
                .font(.caption.weight(.semibold))
                .foregroundStyle(SonderPalette.textLight)
            TextField("https://sonder.example.org", text: $publicText)
                .textFieldStyle(.plain)
                .keyboardType(.URL)
                .textContentType(.URL)
                .autocorrectionDisabled()
                .textInputAutocapitalization(.never)
                .sonderInput()
                .accessibilityIdentifier("server.publicURL")

            Text("Local address (optional)")
                .font(.caption.weight(.semibold))
                .foregroundStyle(SonderPalette.textLight)
            TextField("http://192.168.1.20:8096", text: $localText)
                .textFieldStyle(.plain)
                .keyboardType(.URL)
                .autocorrectionDisabled()
                .textInputAutocapitalization(.never)
                .sonderInput()
                .accessibilityIdentifier("server.localURL")
            Text("Used when your phone can reach it, for faster streaming at home. Otherwise the public address is used.")
                .font(.caption)
                .foregroundStyle(SonderPalette.textLight)
                .fixedSize(horizontal: false, vertical: true)

            DisclosureGroup("Advanced", isExpanded: $showsAdvanced) {
                SecureField("Pairing token (older servers only)", text: $pairingToken)
                    .textFieldStyle(.plain)
                    .autocorrectionDisabled()
                    .textInputAutocapitalization(.never)
                    .sonderInput()
                    .padding(.top, 6)
            }
            .font(.subheadline)
            .tint(SonderPalette.textLight)
        }
    }

    /// Parsed addresses, or nil with a message when the text is not usable.
    static func addresses(publicText: String, localText: String) -> Result<SonderServerAddresses, AddressError> {
        let publicTrimmed = publicText.trimmingCharacters(in: .whitespacesAndNewlines)
        let localTrimmed = localText.trimmingCharacters(in: .whitespacesAndNewlines)
        let publicURL = SonderServerAddresses.parse(publicTrimmed, defaultScheme: "https")
        let localURL = SonderServerAddresses.parse(localTrimmed, defaultScheme: "http")
        if publicTrimmed.isEmpty == false, publicURL == nil { return .failure(.init(message: "The public address is not a valid URL.")) }
        if localTrimmed.isEmpty == false, localURL == nil { return .failure(.init(message: "The local address is not a valid URL.")) }
        if publicURL == nil, localURL == nil { return .failure(.init(message: "Enter the server's public address.")) }
        return .success(SonderServerAddresses(publicURL: publicURL, localURL: localURL))
    }

    struct AddressError: Error { var message: String }
}

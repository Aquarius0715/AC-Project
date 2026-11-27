import AuthenticationServices
import SwiftUI

/// The sign-in screen of both apps: the app's name, one button that opens the provider's page (the web
/// authentication session, without shared cookies, so another account can sign in next), and the last refusal.
public struct SignInView: View {
    let title: String
    let model: SessionModel
    let web: WebSignIn?
    @Environment(\.webAuthenticationSession) private var webAuthenticationSession

    /// `web` replaces the provider's page (the stubbed UI tests); nil opens it.
    public init(title: String, model: SessionModel, web: WebSignIn? = nil) {
        self.title = title
        self.model = model
        self.web = web
    }

    public var body: some View {
        VStack(spacing: Spacing.xl) {
            Spacer()
            Text(verbatim: "AC Project").font(.headline)
            Text(verbatim: title).font(.largeTitle.bold()).multilineTextAlignment(.center)
            if case let .signedOut(message?) = model.state {
                Text(verbatim: message)
                    .font(.callout)
                    .foregroundStyle(Palette.critical)
                    .multilineTextAlignment(.center)
                    .accessibilityIdentifier("signIn.message")
            }
            Spacer()
            Button {
                Task { await model.signIn(using: web ?? open) }
            } label: {
                Group {
                    if model.state == .signingIn {
                        ProgressView()
                    } else {
                        Text(verbatim: tr("Sign in"))
                    }
                }
                .frame(maxWidth: .infinity, minHeight: Size.minTap)
            }
            .primaryAction()
            .disabled(model.state == .signingIn)
            .accessibilityIdentifier("signIn.button")
        }
        .padding(Spacing.xl)
    }

    private func open(_ url: URL, _ scheme: String) async throws -> URL {
        try await webAuthenticationSession.authenticate(
            using: url,
            callback: .customScheme(scheme),
            preferredBrowserSession: .ephemeral,
            additionalHeaderFields: [:]
        )
    }
}

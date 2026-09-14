import { HeyClient } from "../src/index.js";
import {
  HEY_OAUTH_CONFIG,
  generatePKCE,
  generateState,
  authorizationURL,
  exchangeCode,
  refreshToken,
} from "../src/oauth.js";
// Persist state/verifier securely with the login session. Your callback must check
// returned state equals stored state before calling finishLogin.
export async function beginLogin(
  clientId: string,
  redirectUri: string,
  installId: string,
) {
  const config = HEY_OAUTH_CONFIG;
  const pkce = generatePKCE();
  const state = generateState();
  return {
    config,
    pkce,
    state,
    installId,
    url: authorizationURL(config, {
      clientId,
      redirectUri,
      state,
      challenge: pkce.challenge,
      installId,
    }),
  };
}
export async function finishLogin(
  clientId: string,
  redirectUri: string,
  code: string,
  returnedState: string,
  login: Awaited<ReturnType<typeof beginLogin>>,
) {
  if (returnedState !== login.state) throw new Error("OAuth state mismatch");
  let token = await exchangeCode({
    tokenEndpoint: login.config.token_endpoint,
    clientId,
    redirectUri,
    code,
    codeVerifier: login.pkce.verifier,
    installId: login.installId,
  });
  return new HeyClient({
    token: {
      getToken: () => token.access_token,
      refresh: async () => {
        if (!token.refresh_token) throw new Error("No refresh token");
        const next = await refreshToken({
          tokenEndpoint: login.config.token_endpoint,
          clientId,
          refreshToken: token.refresh_token,
          installId: login.installId,
        });
        token = {
          ...next,
          refresh_token: next.refresh_token ?? token.refresh_token,
        };
        // In an application, persist the rotated token in its credential store here.
      },
    },
  });
}

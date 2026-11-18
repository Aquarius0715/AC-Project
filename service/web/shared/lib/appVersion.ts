// The build of this app (IR856): next.config.ts puts the image's APP_VERSION build argument (the deployed tag in the
// shared test environment, IR855) into both the server and the browser bundles. A page whose build differs from the one
// /bff/version answers with was loaded before a deployment and offers a reload. "dev" when the build has none.
export const APP_VERSION = process.env.AC_APP_VERSION ?? "dev";

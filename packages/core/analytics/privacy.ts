// Build policy for this local fork. Environment and server config cannot opt in.
// Keep aligned with server/internal/privacy/reporting.go.
export const EXTERNAL_REPORTING_ENABLED = false;

// Optional marketing metadata and upstream update probes are disabled as well.
// Explicit model, integration, and user-initiated website requests are separate.
export const EXTERNAL_BACKGROUND_REQUESTS_ENABLED = false;

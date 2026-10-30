// Package privacy defines the local fork's reporting policy.
package privacy

// ExternalReportingEnabled is a build policy, not an environment setting.
// Re-enabling external analytics or snapshots requires a reviewed source change.
// It applies only to product reporting, never to model requests, SDK retries,
// task tools, or integrations operating within their existing authorization.
const ExternalReportingEnabled = false

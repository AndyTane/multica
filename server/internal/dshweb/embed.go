package dshweb

import _ "embed"

// Script is embedded so the bridge executable does not depend on the checkout.
//
//go:embed bridge.mjs
var Script string

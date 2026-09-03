package web

import "embed"

// Assets keeps the server-rendered UI independent of the process working
// directory and avoids a Node.js build/runtime dependency.
//
//go:embed templates/*.html static/*
var Assets embed.FS

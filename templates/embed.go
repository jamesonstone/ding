// Package templates embeds the email templates so the ding binary is fully
// self-contained (important for the Lambda single-binary deployment). The
// templates live at the repository root per the project layout, and email
// rendering parses them from FS.
package templates

import "embed"

// FS holds the multipart/alternative email templates.
//
//go:embed email.html.tmpl email.txt.tmpl
var FS embed.FS

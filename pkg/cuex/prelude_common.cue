// Shared constraints for host and package configuration.
// Unified by the Go loader; no cue.mod / module system.

#ID: string & =~"^[a-z]([a-z0-9-]*[a-z0-9])?$"

#Runtime: "js"

#Port: int & >=1 & <=65535

// Package-relative path: no absolute, no ".." segments.
#RelPath: string & =~"^[^/]" & !~"(^|/)\\.\\.($|/)"

// Outbound allowlist entry: hostname, *.hostname, http(s):// origin, or
// bare "*" (allow any http(s) host — explicit open egress; empty egress still denies all).
#Egress: string & =~"^\\*$|^(?:\\*\\.)?[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)+$|^https?://[^\\s/]+$"

#BindingName: string & =~"^[A-Za-z_][A-Za-z0-9_]*$"

#NameBinding: {
	name:     #BindingName
	required: bool | *false
}

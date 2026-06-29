package repository

import "regexp"

// ftsQueryRe matches characters NOT allowed in FTS5 queries.
// Allows Unicode letters (\p{L}), Unicode digits (\p{N}), whitespace, and
// hyphens. This ensures Spanish accented terms (e.g. "alergía", "María")
// pass validation while FTS5 operator characters (*, +, -, NOT, OR, AND)
// are rejected.
var ftsQueryRe = regexp.MustCompile(`[^\p{L}\p{N}\s\-]`)

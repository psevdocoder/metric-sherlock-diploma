package httpapi

import _ "embed"

//go:embed static/target_groups.swagger.json
var targetGroupsSwaggerJSON []byte

package llmproxycontract

// MediaDictionary contains the owned dictionary references and native creation observations.
// Native dictionary identifiers remain private to the gateway.
type MediaDictionary struct {
	DictionaryID         string  `json:"dictionary_id"`
	VersionID            string  `json:"version_id"`
	Provider             string  `json:"provider"`
	Name                 string  `json:"name"`
	CreatedBy            string  `json:"created_by"`
	CreationTimeUnix     int64   `json:"creation_time_unix"`
	VersionRulesNum      int     `json:"version_rules_num"`
	PermissionOnResource *string `json:"permission_on_resource"`
	Description          *string `json:"description"`
}

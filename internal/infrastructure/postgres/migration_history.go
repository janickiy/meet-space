package postgres

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// Historical identities retain provenance without shipping executable old SQL.
// The additional hashes cover reviewed distributions whose SQL differs only in comments.
var historicalMigrationChecksums = map[string][]string{
	"000001_create_record_table.up.sql":                  {"ce196bff9d8d491bfff04bb22ebc487a2b2abf64917102e71b75e189074c1a56"},
	"000002_create_record_segment_table.up.sql":          {"ee3c1be5ec330bcc36635cd4a61977ee2695998dc88b6dea9db921120fe3b643"},
	"000003_create_record_file_table.up.sql":             {"f7763cfc0695089fc4f34b5d4e967725783ca063ec187de6dcf45630fdbb1243"},
	"000004_create_record_event_table.up.sql":            {"2abd962db967a452b50e0496e7ade36963f91c636b9b5730e21d1dca42c84607"},
	"000005_add_worker_id_to_record_table.up.sql":        {"9dc2dc77a9c17471dffa8eb49326d36e8692891a52f3dc4e234cf0e9662ad3ad"},
	"000006_create_users_table.up.sql":                   {"8a57e3c9286ccdc4e2a5a36743b66e77bd5496a9893d6e12592c8aeed84be395"},
	"000007_create_conferences_table.up.sql":             {"021a9684fec198da3023685ea5427b5072ae2e24a6bb8317ddf939a2329fb296"},
	"000008_create_conference_participants_table.up.sql": {"cf741d8c2b70b3df24bc883b2c8ac8bcaba0fecf13ca6595754a697ed200406b"},
	"000009_create_participant_sessions_table.up.sql":    {"29226378579f3f94ba88a0d68990def599af572f98f0afbed1db5d8194a94c88"},
	"000010_conference_recording_control.up.sql":         {"f0d846b1d95d985bfc8b06296d2ead6583790d0fe23d1acc1f9a3ff838f55bcc"},
	"000011_participant_session_media.up.sql":            {"3fdb56c9b3c94c2988d686b8da68046750282f19fab0fb00b90663dee0d3a284"},
	"000012_conference_admission_history.up.sql":         {"07173a4a46291bbca6e9050936b699b8111171a182fd2f04a323e3168ab32b26"},
	"000013_chat_attachments_read.up.sql":                {"114b4a2cdcfcd63dc59e53aa618612386ba43f82b725030d9260d7debd5dd7c6"},
	"000014_notifications.up.sql":                        {"160f57110ee7c9cfe7eb4cfa7b4fe94334e032b21f417fa092fe33f0aed943fe"},
	"000015_background_jobs.up.sql":                      {"a7030e63f834c5d0978172d6fd942c46b2f56d3123a975e05f74e4f12ad07111"},
	"000016_external_integrations.up.sql":                {"8c9d264423fdf412db3d746b7900ab887859710b1ff1f4b3fd056c9c166e8fe1"},
	"000017_recording_content.up.sql":                    {"cb01b27568f878ba043300a448d957bd38294e877e5a43a7aadfc03d4131bd67"},
	"000018_recording_modes.up.sql":                      {"94285d981919c234a08b6bb1707bfa756f950bbf46815af3f6a2ab6cfb94873c"},
	"000019_live_captions_analytics.up.sql":              {"864bf94f0d0a9a3560a7e44a888253bc1cf7dd558a6ea87502bbcc833597bff7"},
	"000020_semantic_search.up.sql":                      {"d12d59a0bd9f337445beacf47397a0f4bfd29e46da0de1ce8abcb8c90a4dbb5e"},
	"000021_admin_capability.up.sql":                     {"a8e2b901aae47b82ac5d9f4e9be1c19d90f95b3b2bcf41b7c84fb4fa38089319"},
	"000022_remove_raised_hands.up.sql":                  {"c75eb88636f11998dbaed6b86f521649f7a552f2ccb5604cbfe7ff43fad6c67b"},
	"000023_guest_sessions.up.sql":                       {"a0a678e48c5d60c3b450d95d3b30419aa0bb489b5abbd0e4e857c5447911055a", "123dccdbdcf3cd09eb231286b8ef504e72bf19e9a98889656cf716996d1b0346"},
	"000024_admit_pending_invite_participants.up.sql":    {"f97591152398959b6b33fa8c237afd801a7b8136d9fd99d48415ee3196a855dd", "b5726bf34ca4ac16792fdec15cf618f04a160bb93d3a47d784374aaf4c349f04"},
	"000025_meeting_invitations.up.sql":                  {"8391a7646275cc016ad469e07180599c70338b2403683a94cb37f462d6222a3f", "f9a4c46254ac53fbd7f97c3f77a7e64ca30ba5920fece953050a8caa907899f7"},
	"000026_persistent_auth_sessions.up.sql":             {"3bf79860d8a8cd75b086f2ba264a1cd74547aac478438fffe7f571ab0b58c675", "57b7beaeab1a7187d0548cca87882cbc0a98de122bd18a6eb827381827921cd7"},
	"000027_recording_page_index.up.sql":                 {"f423a7338d29747be91a5a32422fa568827d34f62cae0dd6186b82f61c117112", "4c562f719766fe3742cee4724bd2ac7c56651a42469a0147c3d9b7f25b7371f1"},
	"000028_personal_conversations.up.sql":               {"6cc7d9f613fd172927696ec46df0ab1a3303f66ddc969ba13772460a7913faff", "30bb53ffc2dcaf18186bf8730a722f848aab65fdee50cb804b1eef5815304253"},
	"000029_conference_chat_actions.up.sql":              {"0e136e6e409bdc2b0fc1e4c020662eab54b4da3a931b13d42147b51d8ab636cf", "409bbc81665b1b684b95af73d1ebb1d9abb4b2662f9e69244f10c089597dbe70"},
	"000030_group_conversations.up.sql":                  {"8b4570f971901439350b42bb2e1f40a5b9b6f049a1e0aac89bb8152b49893b45", "d5b63121fdf3d775e8d889196a5a9931cd4b359616ddbf214e8f4f8e6dcf5735"},
	"000031_conversation_avatar_objects.up.sql":          {"c219cb1bd8e8a39a7b6cdd4ccd93866e28715fd290275c62d23b3b2c63f5cf88", "78f5abd1122c64c93b14d3fb12945e84dfe86d37a7b79bae4eaba5287a35baac"},
	"000032_personal_folders.up.sql":                     {"4db713bbe8e2548a7929314beb063a4c62eb96ce06825f790e18be3bf276cb86"},
	"000033_direct_conversation_preferences.up.sql":      {"cc62424321937609729d49b781dfc9d9dee604cabf7a7f1e29d28f2265a4c3fb"},
	"000034_direct_calls.up.sql":                         {"bcce3fafb5df58a736bf9a61d9ba838c36cefb2a06b5ee1c17a8b426672dfc8a"},
	"000035_conversation_message_pins.up.sql":            {"2c6171d7f103d74550217b9c93bc5e9ca9fa1c2ea7b0c74bcb6b8be236efd8b8", "3ed8b6b976bce7fdaeffcdacc75211d0094b1bd052946d29a73e586baa65bdcb"},
	"000036_user_avatars.up.sql":                         {"8f6af1fd82c12458fa19dd0465125cf55441791200912e64c0e2bf94f287a26d"},
	"000037_remove_speech_features.up.sql":               {"e95e9c60b5787c16904429619543d37b856bceec2f54c44991b1bafeb74a0100"},
	"000038_administrative_console.up.sql":               {"b99c0e7ecbfaf7a06cb0c80b02df00ad67a8f7a88417f05be65a4442aca9154c"},
	"000039_administrator_resource_indexes.up.sql":       {"7e8f8169f3398b7d64fd28ac4f784689ee6a64a9ec79340a9ee4c11601032015"},
	"000040_administrator_search_indexes.up.sql":         {"238919a0dada81db8b08c694b79f255d0f4385c8ecdfdc82933245882a0fa5c2"},
	"000041_platform_settings.up.sql":                    {"44fe8b10a0c19ddd8ab712e85f456779d17cbfa3bafdb57382969e500c50355f"},
	"000042_email_verification.up.sql":                   {"e1e5041afb9b6d0bc91da5f7b7ddc44e817b947fbb25f6536ed7333f884c7123"},
	"000043_dynamic_attachment_size.up.sql":              {"71f2eb334cbff15235d7831218133d5d873cfbf857d07d94c4ab56a860c16f74"},
	"000044_error_events.up.sql":                         {"5de96a2d72efd90db01a1c54ffa49e35e8b1466caa0b3fbdd1bb0fb800128ee7"},
	"000045_password_recovery.up.sql":                    {"72cd93bc1a57a0aacd2afd982ba5c608a232239bdcbb4b87d53a643617754d39"},
	"000046_administrator_login_characters.up.sql":       {"404608d1dca1f0e7892f9b25f1c00c8c8c4fb81f67caee6af78becfe61db15c4"},
	"000047_directory_settings.up.sql":                   {"8ac73804d1705c52a400a0402d990f4b782785473417e8291bccbdab80f4f275"},
	"000048_ldap_accounts.up.sql":                        {"0f3b7bdebe218e77b484eeca167144182f7811d4515a6d633a8fdc76c8588016"},
	"000049_ldap_sync_jobs.up.sql":                       {"8cb55c045305fc009b168f72cc139672986322fe8fd23be8db34d8b61d557b09"},
}

const historicalTableBaselineChecksum = "caca72cd986fa95e71234aeec341fe2ed90963ec410c38fe7a4f8c8afe0cdd7d"
const historicalSchemaChecksum = "4d89a246529aa8d002a95edd282538d76da791aca682f80f505b7c6902dd0585"

var historicalTableNames = []string{
	"administrator_audit_log",
	"administrator_login_limits",
	"administrator_sessions",
	"administrators",
	"auth_legacy_tokens",
	"auth_session_tokens",
	"auth_sessions",
	"background_jobs",
	"calendar_connections",
	"calendar_event_mappings",
	"calendar_oauth_states",
	"chat_attachments",
	"chat_messages",
	"chat_notification_jobs",
	"chat_read_states",
	"conference_chat_pins",
	"conference_chat_preferences",
	"conference_invitations",
	"conference_moderation_audit",
	"conference_participants",
	"conferences",
	"conversation_attachments",
	"conversation_avatar_objects",
	"conversation_members",
	"conversation_message_pins",
	"conversation_messages",
	"conversations",
	"direct_call_slots",
	"direct_calls",
	"email_verifications",
	"error_event_batches",
	"error_events",
	"folder_conferences",
	"folder_conversations",
	"folders",
	"integration_deliveries",
	"ldap_directory_state",
	"ldap_identities",
	"ldap_sync_runs",
	"notification_jobs",
	"notification_preferences",
	"notifications",
	"participant_sessions",
	"password_recoveries",
	"platform_settings",
	"push_devices",
	"record",
	"record_event",
	"record_file",
	"record_segment",
	"recording_outbox",
	"user_avatars",
	"users",
}

// Only numbered entries 1 through 49 belong to this compatibility domain;
// baseline and per-table upgrade identities are validated by their own engine.
func validateHistoricalLedger(applied map[string]string, requireComplete bool) error {
	present := make(map[int]bool)
	for name, checksum := range applied {
		prefix, _, ok := strings.Cut(name, "_")
		if !ok {
			continue
		}
		version, err := strconv.Atoi(prefix)
		if err != nil || version > 49 {
			continue
		}
		allowed, known := historicalMigrationChecksums[name]
		if !known || version < 1 {
			return fmt.Errorf("unknown historical migration %s", name)
		}
		matched := false
		for _, candidate := range allowed {
			matched = matched || checksum == candidate
		}
		if !matched {
			return fmt.Errorf("migration %s checksum mismatch", name)
		}
		present[version] = true
	}
	if requireComplete && len(present) != 49 {
		return fmt.Errorf("historical database requires the complete migration chain 1 through 49; found %d", len(present))
	}
	for version := 1; version <= len(present); version++ {
		if !present[version] {
			return fmt.Errorf("historical migration chain has a gap at %06d", version)
		}
	}
	return nil
}

const historicalSchemaQuery = `WITH selected_tables AS (
 SELECT c.oid,c.relname,c.relkind,c.relrowsecurity,c.relforcerowsecurity
 FROM pg_class c WHERE c.relnamespace=to_regnamespace(quote_ident(current_schema())) AND c.relname IN ?
)
SELECT definition FROM (
 SELECT 'table:'||c.relname||':'||c.relkind::text||':'||c.relrowsecurity||':'||c.relforcerowsecurity AS definition
 FROM selected_tables c
 UNION ALL
 SELECT 'column:'||c.relname||':'||row_number() OVER(PARTITION BY c.oid ORDER BY a.attnum)||':'||a.attname||':'||format_type(a.atttypid,a.atttypmod)||':'||a.attnotnull||':'||a.attidentity::text||':'||a.attgenerated::text||':'||COALESCE(pg_get_expr(d.adbin,d.adrelid),'')
 FROM selected_tables c JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped
 LEFT JOIN pg_attrdef d ON d.adrelid=c.oid AND d.adnum=a.attnum
 WHERE c.relkind='r'
 UNION ALL
 SELECT 'constraint:'||c.relname||':'||con.conname||':'||pg_get_constraintdef(con.oid)||':'||con.convalidated
 FROM pg_constraint con JOIN selected_tables c ON c.oid=con.conrelid
 UNION ALL
 SELECT 'index:'||pg_get_indexdef(i.indexrelid)||':'||i.indisvalid||':'||i.indisready
 FROM pg_index i JOIN selected_tables c ON c.oid=i.indrelid
 UNION ALL
 SELECT 'trigger:'||pg_get_triggerdef(t.oid)||':'||t.tgenabled::text
 FROM pg_trigger t JOIN selected_tables c ON c.oid=t.tgrelid WHERE NOT t.tgisinternal
 UNION ALL
 SELECT 'function:'||pg_get_functiondef(p.oid)
 FROM pg_proc p WHERE p.pronamespace=to_regnamespace(quote_ident(current_schema()))
 AND ((p.proname IN ('check_direct_pair','deny_administrator_audit_mutation','enqueue_admission_notification','enqueue_conference_integration','enqueue_enrollment_integration','enqueue_external_notification','enqueue_recording_notification','set_updated_at','version_conference_integration') AND p.pronargs=0)
 OR (p.proname='nullable_uuid_text' AND p.pronargs=1 AND p.proargtypes='2950'::oidvector))
 UNION ALL
 SELECT 'sequence:'||s.relname||':'||q.seqtypid||':'||q.seqstart||':'||q.seqincrement||':'||q.seqmax||':'||q.seqmin||':'||q.seqcache||':'||q.seqcycle||':'||c.relname||':'||d.refobjsubid
 FROM pg_sequence q JOIN pg_class s ON s.oid=q.seqrelid
 JOIN pg_depend d ON d.objid=s.oid AND d.classid='pg_class'::regclass AND d.deptype='a'
 JOIN selected_tables c ON c.oid=d.refobjid
 WHERE s.relnamespace=to_regnamespace(quote_ident(current_schema()))
) schema_definitions ORDER BY definition`

// PostgreSQL emits qualified names for indexes/functions even when constraints
// use unqualified visible names. Normalize only this schema's quoted identifier.
func historicalSchemaSnapshot(db *gorm.DB, tableNames []string) ([]string, error) {
	var namespace struct{ Name, Quoted string }
	if err := db.Raw("SELECT current_schema() AS name, quote_ident(current_schema()) AS quoted").Scan(&namespace).Error; err != nil {
		return nil, fmt.Errorf("inspect historical schema namespace: %w", err)
	}
	if namespace.Name == "" {
		return nil, fmt.Errorf("historical database has no current schema")
	}
	var definitions []string
	if err := db.Raw(historicalSchemaQuery, tableNames).Scan(&definitions).Error; err != nil {
		return nil, fmt.Errorf("inspect historical schema: %w", err)
	}
	for index := range definitions {
		definitions[index] = strings.ReplaceAll(definitions[index], namespace.Quoted+".", "__meetspace_schema__.")
	}
	sort.Strings(definitions)
	return definitions, nil
}

// Adoption is metadata-only. A complete ledger alone must not bless a missing
// column, disabled integrity trigger, invalid index or modified function body.
func validateHistoricalSchema(db *gorm.DB, tableNames []string) error {
	provided := make(map[string]bool, len(tableNames))
	for _, name := range tableNames {
		provided[name] = true
	}
	for _, name := range historicalTableNames {
		if !provided[name] {
			return fmt.Errorf("historical table %s is missing from the migration plan", name)
		}
	}
	definitions, err := historicalSchemaSnapshot(db, historicalTableNames)
	if err != nil {
		return err
	}
	actual := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(definitions, "\n")+"\n")))
	if actual != historicalSchemaChecksum {
		return fmt.Errorf("historical database schema differs from version 49; no migration history was adopted")
	}
	return nil
}

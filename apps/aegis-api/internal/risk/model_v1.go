package risk

import (
	"company.local/aegis/apps/aegis-api/internal/domain"
	"encoding/json"
	"slices"
	"strings"
)

// Frozen exact Appendix-A tuple registry. No category wildcard or user rules.
var tuplesV1 = []domain.RiskTechnicalTuple{
	{Category: "aaa", Subcategory: "global", ClassificationRule: "aaa.global", RuleID: "risk.tech.aaa.v1", Impact: "high"},
	{Category: "aaa", Subcategory: "line_auth", ClassificationRule: "aaa.line_auth", RuleID: "risk.tech.aaa.v1", Impact: "high"},
	{Category: "aaa", Subcategory: "local_user", ClassificationRule: "aaa.local_user", RuleID: "risk.tech.aaa.v1", Impact: "high"},
	{Category: "aaa", Subcategory: "radius", ClassificationRule: "aaa.radius", RuleID: "risk.tech.aaa.v1", Impact: "high"},
	{Category: "aaa", Subcategory: "radius", ClassificationRule: "aaa.radius.context", RuleID: "risk.tech.aaa.v1", Impact: "high"},
	{Category: "aaa", Subcategory: "tacacs", ClassificationRule: "aaa.tacacs", RuleID: "risk.tech.aaa.v1", Impact: "high"},
	{Category: "aaa", Subcategory: "tacacs", ClassificationRule: "aaa.tacacs.context", RuleID: "risk.tech.aaa.v1", Impact: "high"},
	{Category: "access_control", Subcategory: "acl", ClassificationRule: "access_control.acl", RuleID: "risk.tech.access_control.v1", Impact: "high"},
	{Category: "access_control", Subcategory: "acl", ClassificationRule: "access_control.acl.context", RuleID: "risk.tech.access_control.v1", Impact: "high"},
	{Category: "access_control", Subcategory: "acl_binding", ClassificationRule: "access_control.acl_binding", RuleID: "risk.tech.access_control.v1", Impact: "high"},
	{Category: "access_control", Subcategory: "control_plane", ClassificationRule: "access_control.control_plane", RuleID: "risk.tech.access_control.v1", Impact: "high"},
	{Category: "access_control", Subcategory: "control_plane", ClassificationRule: "access_control.control_plane.context", RuleID: "risk.tech.access_control.v1", Impact: "high"},
	{Category: "access_control", Subcategory: "object_group", ClassificationRule: "access_control.object_group", RuleID: "risk.tech.access_control.v1", Impact: "high"},
	{Category: "access_control", Subcategory: "object_group", ClassificationRule: "access_control.object_group.context", RuleID: "risk.tech.access_control.v1", Impact: "high"},
	{Category: "crypto_vpn", Subcategory: "crypto_map", ClassificationRule: "crypto_vpn.crypto_map", RuleID: "risk.tech.crypto_vpn.v1", Impact: "high"},
	{Category: "crypto_vpn", Subcategory: "crypto_map", ClassificationRule: "crypto_vpn.crypto_map.context", RuleID: "risk.tech.crypto_vpn.v1", Impact: "high"},
	{Category: "crypto_vpn", Subcategory: "generic", ClassificationRule: "crypto_vpn.generic", RuleID: "risk.tech.crypto_vpn.v1", Impact: "high"},
	{Category: "crypto_vpn", Subcategory: "ike_isakmp", ClassificationRule: "crypto_vpn.ike_isakmp", RuleID: "risk.tech.crypto_vpn.v1", Impact: "high"},
	{Category: "crypto_vpn", Subcategory: "ike_isakmp", ClassificationRule: "crypto_vpn.ike_isakmp.context", RuleID: "risk.tech.crypto_vpn.v1", Impact: "high"},
	{Category: "crypto_vpn", Subcategory: "ipsec", ClassificationRule: "crypto_vpn.ipsec", RuleID: "risk.tech.crypto_vpn.v1", Impact: "high"},
	{Category: "crypto_vpn", Subcategory: "ipsec", ClassificationRule: "crypto_vpn.ipsec.context", RuleID: "risk.tech.crypto_vpn.v1", Impact: "high"},
	{Category: "crypto_vpn", Subcategory: "tunnel", ClassificationRule: "crypto_vpn.tunnel", RuleID: "risk.tech.crypto_vpn.v1", Impact: "high"},
	{Category: "interface", Subcategory: "addressing", ClassificationRule: "interface.addressing", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "interface", Subcategory: "admin_state", ClassificationRule: "interface.admin_state", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "interface", Subcategory: "definition", ClassificationRule: "interface.definition", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "interface", Subcategory: "description", ClassificationRule: "interface.description", RuleID: "risk.tech.interface_description.v1", Impact: "low"},
	{Category: "interface", Subcategory: "generic", ClassificationRule: "interface.generic", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "management", Subcategory: "console", ClassificationRule: "management.console", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "management", Subcategory: "console", ClassificationRule: "management.console.context", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "management", Subcategory: "dns", ClassificationRule: "management.dns", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "management", Subcategory: "ntp", ClassificationRule: "management.ntp", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "management", Subcategory: "remote_access", ClassificationRule: "management.remote_access", RuleID: "risk.tech.management_access.v1", Impact: "high"},
	{Category: "management", Subcategory: "remote_access", ClassificationRule: "management.remote_access.context", RuleID: "risk.tech.management_access.v1", Impact: "high"},
	{Category: "management", Subcategory: "web", ClassificationRule: "management.web", RuleID: "risk.tech.management_access.v1", Impact: "high"},
	{Category: "monitoring", Subcategory: "flow", ClassificationRule: "monitoring.flow", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "monitoring", Subcategory: "flow", ClassificationRule: "monitoring.flow.context", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "monitoring", Subcategory: "logging", ClassificationRule: "monitoring.logging", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "monitoring", Subcategory: "snmp", ClassificationRule: "monitoring.snmp", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "monitoring", Subcategory: "telemetry", ClassificationRule: "monitoring.telemetry", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "monitoring", Subcategory: "telemetry", ClassificationRule: "monitoring.telemetry.context", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "other", Subcategory: "unclassified", ClassificationRule: "fallback.unclassified", RuleID: "risk.tech.unknown_item.v1", Impact: "unknown"},
	{Category: "qos", Subcategory: "class_map", ClassificationRule: "qos.class_map", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "qos", Subcategory: "class_map", ClassificationRule: "qos.class_map.context", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "qos", Subcategory: "generic", ClassificationRule: "qos.generic", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "qos", Subcategory: "policy_map", ClassificationRule: "qos.policy_map", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "qos", Subcategory: "policy_map", ClassificationRule: "qos.policy_map.context", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "qos", Subcategory: "service_policy", ClassificationRule: "qos.service_policy", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "redundancy", Subcategory: "glbp", ClassificationRule: "redundancy.glbp", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "redundancy", Subcategory: "hsrp", ClassificationRule: "redundancy.hsrp", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "redundancy", Subcategory: "vrrp", ClassificationRule: "redundancy.vrrp", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "routing", Subcategory: "bgp", ClassificationRule: "routing.bgp", RuleID: "risk.tech.routing.v1", Impact: "high"},
	{Category: "routing", Subcategory: "bgp", ClassificationRule: "routing.bgp.context", RuleID: "risk.tech.routing.v1", Impact: "high"},
	{Category: "routing", Subcategory: "eigrp", ClassificationRule: "routing.eigrp", RuleID: "risk.tech.routing.v1", Impact: "high"},
	{Category: "routing", Subcategory: "eigrp", ClassificationRule: "routing.eigrp.context", RuleID: "risk.tech.routing.v1", Impact: "high"},
	{Category: "routing", Subcategory: "generic", ClassificationRule: "routing.generic", RuleID: "risk.tech.routing.v1", Impact: "high"},
	{Category: "routing", Subcategory: "generic", ClassificationRule: "routing.generic.context", RuleID: "risk.tech.routing.v1", Impact: "high"},
	{Category: "routing", Subcategory: "isis", ClassificationRule: "routing.isis", RuleID: "risk.tech.routing.v1", Impact: "high"},
	{Category: "routing", Subcategory: "isis", ClassificationRule: "routing.isis.context", RuleID: "risk.tech.routing.v1", Impact: "high"},
	{Category: "routing", Subcategory: "ospf", ClassificationRule: "routing.ospf", RuleID: "risk.tech.routing.v1", Impact: "high"},
	{Category: "routing", Subcategory: "ospf", ClassificationRule: "routing.ospf.context", RuleID: "risk.tech.routing.v1", Impact: "high"},
	{Category: "routing", Subcategory: "rip", ClassificationRule: "routing.rip", RuleID: "risk.tech.routing.v1", Impact: "high"},
	{Category: "routing", Subcategory: "rip", ClassificationRule: "routing.rip.context", RuleID: "risk.tech.routing.v1", Impact: "high"},
	{Category: "routing", Subcategory: "route_filter", ClassificationRule: "routing.route_filter", RuleID: "risk.tech.routing.v1", Impact: "high"},
	{Category: "routing", Subcategory: "route_policy", ClassificationRule: "routing.route_policy", RuleID: "risk.tech.routing.v1", Impact: "high"},
	{Category: "routing", Subcategory: "route_policy", ClassificationRule: "routing.route_policy.context", RuleID: "risk.tech.routing.v1", Impact: "high"},
	{Category: "routing", Subcategory: "static_route", ClassificationRule: "routing.static_route", RuleID: "risk.tech.routing.v1", Impact: "high"},
	{Category: "services", Subcategory: "dhcp", ClassificationRule: "services.dhcp", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "services", Subcategory: "dhcp", ClassificationRule: "services.dhcp.context", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "services", Subcategory: "ip_sla", ClassificationRule: "services.ip_sla", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "services", Subcategory: "ip_sla", ClassificationRule: "services.ip_sla.context", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "services", Subcategory: "nat", ClassificationRule: "services.nat", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "services", Subcategory: "tracking", ClassificationRule: "services.tracking", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "services", Subcategory: "tracking", ClassificationRule: "services.tracking.context", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "switching", Subcategory: "etherchannel", ClassificationRule: "switching.etherchannel", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "switching", Subcategory: "spanning_tree", ClassificationRule: "switching.spanning_tree", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "switching", Subcategory: "switchport", ClassificationRule: "switching.switchport", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "switching", Subcategory: "vlan", ClassificationRule: "switching.vlan", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "switching", Subcategory: "vlan", ClassificationRule: "switching.vlan.context", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "switching", Subcategory: "vtp", ClassificationRule: "switching.vtp", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "system", Subcategory: "boot", ClassificationRule: "system.boot", RuleID: "risk.tech.system_control.v1", Impact: "high"},
	{Category: "system", Subcategory: "crypto_key", ClassificationRule: "system.crypto_key", RuleID: "risk.tech.system_control.v1", Impact: "high"},
	{Category: "system", Subcategory: "global_service", ClassificationRule: "system.global_service", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
	{Category: "system", Subcategory: "identity", ClassificationRule: "system.hostname", RuleID: "risk.tech.known_category.v1", Impact: "medium"},
}

// DefinitionV1 returns detached copies; callers cannot mutate the built-in model.
func DefinitionV1() domain.RiskDefinition {
	d := domain.RiskDefinition{
		ModelVersion: 1, SchemaVersion: 1, EngineVersion: 1, MaterializerVersion: 1,
		AcceptedAnalysisVersions: []int{1}, AcceptedClassificationVersions: []int{1},
		Aggregation: domain.RiskAggregationContract{
			ImpactMode: "unknown_dominates_with_known_floor", PriorityMode: "maximum_floor",
			FindingOrder: "kind_ordinal_rule_subject_reason", DuplicateMode: "reject",
			NoItemsImpact: "unknown", DefaultReviewPriority: "normal", CriticalEnabled: false,
		},
		TechnicalRules: slices.Clone(tuplesV1),
		PriorityRules: []domain.RiskPriorityRule{
			{RuleID: "risk.gov.policy_violation.v1", Predicate: "policy_violation", Floor: "urgent"},
			{RuleID: "risk.gov.authorization_revoked.v1", Predicate: "authorization_revoked", Floor: "urgent"},
			{RuleID: "risk.gov.unaccounted.v1", Predicate: "unaccounted", Floor: "elevated"},
			{RuleID: "risk.gov.policy_not_evaluated.v1", Predicate: "policy_not_evaluated", Floor: "elevated"},
			{RuleID: "risk.evidence.policy_uncovered.v1", Predicate: "policy_uncovered", Floor: "urgent"},
			{RuleID: "risk.evidence.partial.v1", Predicate: "partial", Floor: "elevated"},
			{RuleID: "risk.tech.high.v1", Predicate: "known_high", Floor: "elevated"},
		},
	}
	explanations := map[string]string{
		"risk.evidence.no_items.v1":           "No structured items justify a technical impact class.",
		"risk.evidence.unsupported.v1":        "The source version is not eligible for this model.",
		"risk.evidence.partial.v1":            "Evidence is partial under the v1 contract, not a device-security assessment.",
		"risk.evidence.policy_uncovered.v1":   "Some recorded policy items are uncovered.",
		"risk.summary.multiple_categories.v1": "Multiple metadata categories are affected; this is not a numeric score.",
		"risk.summary.normal_review.v1":       "Normal review priority is not approval or a safety guarantee.",
		"risk.gov.authorized_recorded.v1":     "Recorded authorization is a historical fact, not automatic approval.",
		"risk.gov.policy_pass_recorded.v1":    "Recorded policy PASS covers the evaluated items, not full-device security.",
		"risk.gov.policy_violation.v1":        "Recorded policy VIOLATION requires urgent human review.",
		"risk.gov.authorization_revoked.v1":   "The linked ticket was recorded revoked at capture.",
		"risk.gov.unaccounted.v1":             "The observed change is unaccounted for by this workflow.",
		"risk.gov.policy_not_evaluated.v1":    "No final recorded policy evidence was available.",
		"risk.tech.high.v1":                   "A high known impact indication requires elevated human review.",
	}
	for _, t := range d.TechnicalRules {
		explanations[t.RuleID] = "Potential " + t.Impact + " technical impact under limited verified classification metadata; not measured outage or exploitability."
	}
	// A rule may match multiple tuples; its wording must not depend on iteration.
	for id, text := range explanations {
		d.ExplanationTemplates = append(d.ExplanationTemplates, domain.RiskExplanationTemplate{ReasonCode: id, Text: text})
	}
	slices.SortFunc(d.TechnicalRules, func(a, b domain.RiskTechnicalTuple) int {
		return strings.Compare(a.ClassificationRule, b.ClassificationRule)
	})
	slices.SortFunc(d.PriorityRules, func(a, b domain.RiskPriorityRule) int { return strings.Compare(a.RuleID, b.RuleID) })
	slices.SortFunc(d.ExplanationTemplates, func(a, b domain.RiskExplanationTemplate) int { return strings.Compare(a.ReasonCode, b.ReasonCode) })
	return d
}
func CanonicalDefinitionV1() (string, string) {
	data, err := json.Marshal(DefinitionV1())
	if err != nil {
		panic("risk definition encoding failed")
	}
	return string(data), Digest("aegis-risk-definition-v1", data)
}

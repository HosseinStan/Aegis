package changes

import (
	"strings"

	"company.local/aegis/apps/aegis-api/internal/domain"
)

// Classification describes technical meaning only, independent of operation,
// sensitivity, authorization, policy, and time. RuleIDs are fixed literals.
type Classification struct {
	Category    string
	Subcategory string
	RuleID      string
}

// Classify consumes sanitized item data. Matching normalization does not mutate
// either input, and unknown or malformed syntax always has a valid fallback.
func Classify(contextPath []string, command string) Classification {
	tokens := classificationTokens(command)
	parents := make([][]string, len(contextPath))
	for i, parent := range contextPath {
		parents[i] = classificationTokens(parent)
	}

	// Explicit exception: control-plane bindings are not ordinary interface QoS.
	if tokenPrefix(tokens, "service-policy") && hasAncestor(parents, "control-plane") {
		return Classification{domain.ChangeCategoryAccessControl, "control_plane", "access_control.control_plane.context"}
	}
	// Specific command families take precedence over inherited/generic context.
	if result, ok := matchClassification(commandClassificationRules, tokens); ok {
		return result
	}

	// Context-specific command meanings precede broad parent inheritance.
	if hasAncestor(parents, "line") {
		if tokenPrefix(tokens, "login") || tokenPrefix(tokens, "password") ||
			tokenPrefix(tokens, "authorization") || tokenPrefix(tokens, "accounting") {
			return Classification{domain.ChangeCategoryAAA, "line_auth", "aaa.line_auth"}
		}
		if tokenPrefix(tokens, "transport", "input") || tokenPrefix(tokens, "transport", "output") {
			return Classification{domain.ChangeCategoryManagement, "remote_access", "management.remote_access"}
		}
	}
	if hasAncestor(parents, "interface") {
		switch {
		case tokenPrefix(tokens, "description"):
			return Classification{domain.ChangeCategoryInterface, "description", "interface.description"}
		case tokenPrefix(tokens, "shutdown"):
			return Classification{domain.ChangeCategoryInterface, "admin_state", "interface.admin_state"}
		case tokenPrefix(tokens, "ip", "address"), tokenPrefix(tokens, "ipv6", "address"), tokenPrefix(tokens, "ip", "unnumbered"):
			return Classification{domain.ChangeCategoryInterface, "addressing", "interface.addressing"}
		}
	}

	// Inspect the entire hierarchy, nearest recognized context first. Generic
	// interface inheritance is deliberately excluded until specific families
	// and other recognized ancestors have had a chance to match.
	for i := len(parents) - 1; i >= 0; i-- {
		if result, ok := matchClassification(contextClassificationRules, parents[i]); ok {
			return result
		}
	}
	if hasAncestor(parents, "router") || hasAncestor(parents, "ipv6", "router") {
		return Classification{domain.ChangeCategoryRouting, "generic", "routing.generic.context"}
	}
	if hasAncestor(parents, "interface") {
		return Classification{domain.ChangeCategoryInterface, "generic", "interface.generic"}
	}
	if result, ok := matchClassification(globalClassificationRules, tokens); ok {
		return result
	}
	return Classification{domain.ChangeCategoryOther, "unclassified", "fallback.unclassified"}
}

type classificationRule struct {
	result   Classification
	prefixes [][]string
}

func classificationRuleFor(category, subcategory, id string, prefixes ...string) classificationRule {
	rule := classificationRule{result: Classification{category, subcategory, id}}
	for _, prefix := range prefixes {
		rule.prefixes = append(rule.prefixes, strings.Fields(prefix))
	}
	return rule
}

// The rule tables are compile-time Go definitions, not runtime configuration.
// Order is significant: narrow command families precede broad generic rules.
var commandClassificationRules = []classificationRule{
	classificationRuleFor(domain.ChangeCategorySystem, "identity", "system.hostname", "hostname"),
	classificationRuleFor(domain.ChangeCategorySystem, "boot", "system.boot", "boot"),
	classificationRuleFor(domain.ChangeCategorySystem, "crypto_key", "system.crypto_key", "crypto key"),
	classificationRuleFor(domain.ChangeCategoryInterface, "definition", "interface.definition", "interface"),
	classificationRuleFor(domain.ChangeCategorySwitching, "vlan", "switching.vlan", "vlan"),
	classificationRuleFor(domain.ChangeCategorySwitching, "switchport", "switching.switchport", "switchport"),
	classificationRuleFor(domain.ChangeCategorySwitching, "spanning_tree", "switching.spanning_tree", "spanning-tree"),
	classificationRuleFor(domain.ChangeCategorySwitching, "etherchannel", "switching.etherchannel", "channel-group", "port-channel"),
	classificationRuleFor(domain.ChangeCategorySwitching, "vtp", "switching.vtp", "vtp"),
	classificationRuleFor(domain.ChangeCategoryRouting, "static_route", "routing.static_route", "ip route", "ipv6 route"),
	classificationRuleFor(domain.ChangeCategoryRouting, "bgp", "routing.bgp", "router bgp"),
	classificationRuleFor(domain.ChangeCategoryRouting, "ospf", "routing.ospf", "router ospf", "router ospfv3", "ipv6 router ospf", "ip ospf", "ipv6 ospf"),
	classificationRuleFor(domain.ChangeCategoryRouting, "eigrp", "routing.eigrp", "router eigrp"),
	classificationRuleFor(domain.ChangeCategoryRouting, "rip", "routing.rip", "router rip"),
	classificationRuleFor(domain.ChangeCategoryRouting, "isis", "routing.isis", "router isis"),
	classificationRuleFor(domain.ChangeCategoryRouting, "route_policy", "routing.route_policy", "route-map"),
	classificationRuleFor(domain.ChangeCategoryRouting, "route_filter", "routing.route_filter", "ip prefix-list", "ipv6 prefix-list"),
	classificationRuleFor(domain.ChangeCategoryAccessControl, "acl", "access_control.acl", "access-list", "ip access-list", "ipv6 access-list"),
	classificationRuleFor(domain.ChangeCategoryAccessControl, "acl_binding", "access_control.acl_binding", "ip access-group", "ipv6 traffic-filter"),
	classificationRuleFor(domain.ChangeCategoryAccessControl, "object_group", "access_control.object_group", "object-group"),
	classificationRuleFor(domain.ChangeCategoryAccessControl, "control_plane", "access_control.control_plane", "control-plane"),
	classificationRuleFor(domain.ChangeCategoryAAA, "global", "aaa.global", "aaa", "enable secret", "enable password"),
	classificationRuleFor(domain.ChangeCategoryAAA, "local_user", "aaa.local_user", "username"),
	classificationRuleFor(domain.ChangeCategoryAAA, "radius", "aaa.radius", "radius server", "radius-server"),
	classificationRuleFor(domain.ChangeCategoryAAA, "tacacs", "aaa.tacacs", "tacacs server", "tacacs-server"),
	classificationRuleFor(domain.ChangeCategoryManagement, "remote_access", "management.remote_access", "line vty", "ip ssh", "ssh"),
	classificationRuleFor(domain.ChangeCategoryManagement, "console", "management.console", "line console"),
	classificationRuleFor(domain.ChangeCategoryManagement, "web", "management.web", "ip http"),
	classificationRuleFor(domain.ChangeCategoryManagement, "dns", "management.dns", "ip domain-name", "ip name-server"),
	classificationRuleFor(domain.ChangeCategoryManagement, "ntp", "management.ntp", "ntp"),
	classificationRuleFor(domain.ChangeCategoryMonitoring, "snmp", "monitoring.snmp", "snmp-server"),
	classificationRuleFor(domain.ChangeCategoryMonitoring, "logging", "monitoring.logging", "logging"),
	classificationRuleFor(domain.ChangeCategoryMonitoring, "telemetry", "monitoring.telemetry", "telemetry"),
	classificationRuleFor(domain.ChangeCategoryMonitoring, "flow", "monitoring.flow", "flow exporter", "flow monitor", "ip flow", "netflow"),
	classificationRuleFor(domain.ChangeCategoryServices, "dhcp", "services.dhcp", "ip dhcp", "service dhcp", "ip helper-address"),
	classificationRuleFor(domain.ChangeCategoryServices, "nat", "services.nat", "ip nat"),
	classificationRuleFor(domain.ChangeCategoryServices, "ip_sla", "services.ip_sla", "ip sla"),
	classificationRuleFor(domain.ChangeCategoryServices, "tracking", "services.tracking", "track"),
	classificationRuleFor(domain.ChangeCategoryCryptoVPN, "ike_isakmp", "crypto_vpn.ike_isakmp", "crypto isakmp", "crypto ikev2"),
	classificationRuleFor(domain.ChangeCategoryCryptoVPN, "ipsec", "crypto_vpn.ipsec", "crypto ipsec"),
	classificationRuleFor(domain.ChangeCategoryCryptoVPN, "crypto_map", "crypto_vpn.crypto_map", "crypto map"),
	classificationRuleFor(domain.ChangeCategoryCryptoVPN, "tunnel", "crypto_vpn.tunnel", "tunnel protection ipsec", "tunnel mode ipsec", "tunnel mode gre", "tunnel source", "tunnel destination", "tunnel key"),
	classificationRuleFor(domain.ChangeCategoryCryptoVPN, "generic", "crypto_vpn.generic", "crypto dynamic-map", "crypto vpn"),
	classificationRuleFor(domain.ChangeCategoryQoS, "class_map", "qos.class_map", "class-map"),
	classificationRuleFor(domain.ChangeCategoryQoS, "policy_map", "qos.policy_map", "policy-map"),
	classificationRuleFor(domain.ChangeCategoryQoS, "service_policy", "qos.service_policy", "service-policy"),
	classificationRuleFor(domain.ChangeCategoryQoS, "generic", "qos.generic", "qos", "mls qos"),
	classificationRuleFor(domain.ChangeCategoryRedundancy, "hsrp", "redundancy.hsrp", "standby"),
	classificationRuleFor(domain.ChangeCategoryRedundancy, "vrrp", "redundancy.vrrp", "vrrp"),
	classificationRuleFor(domain.ChangeCategoryRedundancy, "glbp", "redundancy.glbp", "glbp"),
}

var contextClassificationRules = []classificationRule{
	classificationRuleFor(domain.ChangeCategorySwitching, "vlan", "switching.vlan.context", "vlan"),
	classificationRuleFor(domain.ChangeCategoryRouting, "bgp", "routing.bgp.context", "router bgp"),
	classificationRuleFor(domain.ChangeCategoryRouting, "ospf", "routing.ospf.context", "router ospf", "router ospfv3", "ipv6 router ospf"),
	classificationRuleFor(domain.ChangeCategoryRouting, "eigrp", "routing.eigrp.context", "router eigrp"),
	classificationRuleFor(domain.ChangeCategoryRouting, "rip", "routing.rip.context", "router rip"),
	classificationRuleFor(domain.ChangeCategoryRouting, "isis", "routing.isis.context", "router isis"),
	classificationRuleFor(domain.ChangeCategoryRouting, "route_policy", "routing.route_policy.context", "route-map"),
	classificationRuleFor(domain.ChangeCategoryAccessControl, "acl", "access_control.acl.context", "access-list", "ip access-list", "ipv6 access-list"),
	classificationRuleFor(domain.ChangeCategoryAccessControl, "object_group", "access_control.object_group.context", "object-group"),
	classificationRuleFor(domain.ChangeCategoryAccessControl, "control_plane", "access_control.control_plane.context", "control-plane"),
	classificationRuleFor(domain.ChangeCategoryAAA, "radius", "aaa.radius.context", "radius server", "radius-server"),
	classificationRuleFor(domain.ChangeCategoryAAA, "tacacs", "aaa.tacacs.context", "tacacs server", "tacacs-server"),
	classificationRuleFor(domain.ChangeCategoryManagement, "console", "management.console.context", "line console"),
	classificationRuleFor(domain.ChangeCategoryManagement, "remote_access", "management.remote_access.context", "line"),
	classificationRuleFor(domain.ChangeCategoryMonitoring, "telemetry", "monitoring.telemetry.context", "telemetry"),
	classificationRuleFor(domain.ChangeCategoryMonitoring, "flow", "monitoring.flow.context", "flow exporter", "flow monitor"),
	classificationRuleFor(domain.ChangeCategoryServices, "dhcp", "services.dhcp.context", "ip dhcp"),
	classificationRuleFor(domain.ChangeCategoryServices, "ip_sla", "services.ip_sla.context", "ip sla"),
	classificationRuleFor(domain.ChangeCategoryServices, "tracking", "services.tracking.context", "track"),
	classificationRuleFor(domain.ChangeCategoryCryptoVPN, "ike_isakmp", "crypto_vpn.ike_isakmp.context", "crypto isakmp", "crypto ikev2"),
	classificationRuleFor(domain.ChangeCategoryCryptoVPN, "ipsec", "crypto_vpn.ipsec.context", "crypto ipsec"),
	classificationRuleFor(domain.ChangeCategoryCryptoVPN, "crypto_map", "crypto_vpn.crypto_map.context", "crypto map"),
	classificationRuleFor(domain.ChangeCategoryQoS, "class_map", "qos.class_map.context", "class-map"),
	classificationRuleFor(domain.ChangeCategoryQoS, "policy_map", "qos.policy_map.context", "policy-map"),
}

var globalClassificationRules = []classificationRule{
	classificationRuleFor(domain.ChangeCategorySystem, "global_service", "system.global_service", "service"),
	classificationRuleFor(domain.ChangeCategoryRouting, "generic", "routing.generic", "router", "ipv6 router"),
}

func classificationTokens(text string) []string {
	tokens := strings.Fields(strings.ToLower(text))
	if len(tokens) > 0 && tokens[0] == "no" {
		tokens = tokens[1:]
	}
	return tokens
}

func tokenPrefix(tokens []string, prefix ...string) bool {
	if len(tokens) < len(prefix) {
		return false
	}
	for i, token := range prefix {
		if tokens[i] != token {
			return false
		}
	}
	return true
}

func hasAncestor(parents [][]string, prefix ...string) bool {
	for _, parent := range parents {
		if tokenPrefix(parent, prefix...) {
			return true
		}
	}
	return false
}

func matchClassification(rules []classificationRule, tokens []string) (Classification, bool) {
	for _, rule := range rules {
		for _, prefix := range rule.prefixes {
			if tokenPrefix(tokens, prefix...) {
				return rule.result, true
			}
		}
	}
	return Classification{}, false
}

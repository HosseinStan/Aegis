package domain

// Classification and structural analysis are independently versioned.
const ChangeClassificationVersion = 1

const (
	ChangeCategoryUnclassified  = "unclassified"
	ChangeCategorySystem        = "system"
	ChangeCategoryInterface     = "interface"
	ChangeCategorySwitching     = "switching"
	ChangeCategoryRouting       = "routing"
	ChangeCategoryAccessControl = "access_control"
	ChangeCategoryAAA           = "aaa"
	ChangeCategoryManagement    = "management"
	ChangeCategoryMonitoring    = "monitoring"
	ChangeCategoryServices      = "services"
	ChangeCategoryCryptoVPN     = "crypto_vpn"
	ChangeCategoryQoS           = "qos"
	ChangeCategoryRedundancy    = "redundancy"
	ChangeCategoryOther         = "other"
)

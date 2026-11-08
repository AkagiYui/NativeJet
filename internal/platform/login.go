package platform

type LoginItemStatus int

const (
	// These values mirror SMAppServiceStatus for registered apps.
	LoginItemOff LoginItemStatus = iota
	LoginItemOn
	LoginItemRequiresApproval
	LoginItemUnavailable LoginItemStatus = -1
)

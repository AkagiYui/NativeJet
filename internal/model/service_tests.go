package model

type ServiceAvailability string

const (
	ServiceUnknown   ServiceAvailability = "unknown"
	ServiceAvailable ServiceAvailability = "available"
	ServicePartial   ServiceAvailability = "partial"
	ServiceBlocked   ServiceAvailability = "blocked"
	ServiceReachable ServiceAvailability = "reachable"
)

type ServiceTestResult struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Availability  ServiceAvailability `json:"availability"`
	RegionSupport ServiceAvailability `json:"regionSupport,omitempty"`
	Evidence      string              `json:"evidence,omitempty"`
	Detail        string              `json:"detail,omitempty"`
	ElapsedMS     int64               `json:"elapsedMs,omitempty"`
	Region        string              `json:"region,omitempty"`
	Route         []string            `json:"route,omitempty"`
}

type ServiceTests struct {
	Results []ServiceTestResult `json:"results"`
	Problem string              `json:"problem,omitempty"`
}

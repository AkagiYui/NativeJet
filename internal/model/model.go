package model

import (
	"encoding/json"
	"time"
)

const ProtocolVersion = 1

type Profile struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Kind             string    `json:"kind"`
	URL              string    `json:"url,omitempty"`
	Content          string    `json:"content"`
	Override         string    `json:"override"`
	UpdatedAt        time.Time `json:"updatedAt"`
	IntervalHours    int       `json:"intervalHours"`
	SubscriptionInfo string    `json:"subscriptionInfo,omitempty"`
	ETag             string    `json:"etag,omitempty"`
}

type GlobalOverride struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

func DefaultGlobalOverride(content string) GlobalOverride {
	return GlobalOverride{ID: "000000000000000000000000", Name: "默认", Content: content}
}

type Kernel struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Version   string `json:"version"`
	Arch      string `json:"arch"`
	Error     string `json:"error,omitempty"`
	Installed bool   `json:"installed"`
}

type Revision struct {
	ID                 string    `json:"id"`
	ProfileID          string    `json:"profileId"`
	KernelID           string    `json:"kernelId"`
	CreatedAt          time.Time `json:"createdAt"`
	Hash               string    `json:"hash"`
	Source             string    `json:"source"`
	GlobalOverride     string    `json:"globalOverride"`
	GlobalOverrideID   string    `json:"globalOverrideId,omitempty"`
	GlobalOverrideName string    `json:"globalOverrideName,omitempty"`
	ProfileOverride    string    `json:"profileOverride"`
}

type ProcessRecord struct {
	PID                int    `json:"pid"`
	Path               string `json:"path"`
	StartTime          int64  `json:"startTime"`
	RevisionID         string `json:"revisionId"`
	ControllerOccupied bool   `json:"controllerOccupied,omitempty"`
}

type State struct {
	Version                int              `json:"version"`
	Profiles               []Profile        `json:"profiles"`
	GlobalOverrides        []GlobalOverride `json:"globalOverrides"`
	SelectedGlobalOverride string           `json:"selectedGlobalOverride"`
	SelectedKernel         string           `json:"selectedKernel"`
	ActiveProfile          string           `json:"activeProfile"`
	ActiveRevision         string           `json:"activeRevision"`
	Revisions              []Revision       `json:"revisions"`
	Installed              []Kernel         `json:"installed"`
	Process                *ProcessRecord   `json:"process,omitempty"`
}

func (s *State) UnmarshalJSON(data []byte) error {
	type state State
	var wire struct {
		state
		LegacyOverride *string `json:"globalOverride"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*s = State(wire.state)
	if s.GlobalOverrides == nil && s.SelectedGlobalOverride == "" && wire.LegacyOverride != nil {
		preset := DefaultGlobalOverride(*wire.LegacyOverride)
		s.GlobalOverrides = []GlobalOverride{preset}
		s.SelectedGlobalOverride = preset.ID
	}
	return nil
}

func (s *State) GlobalOverrideByID(id string) *GlobalOverride {
	for i := range s.GlobalOverrides {
		if s.GlobalOverrides[i].ID == id {
			return &s.GlobalOverrides[i]
		}
	}
	return nil
}

type Trace struct {
	Layer   string `json:"layer"`
	Value   any    `json:"value"`
	Line    int    `json:"line,omitempty"`
	Deleted bool   `json:"deleted,omitempty"`
}

type Field struct {
	Path   string  `json:"path"`
	Value  any     `json:"value"`
	Source string  `json:"source"`
	Trace  []Trace `json:"trace"`
}

type Preview struct {
	YAML   string  `json:"yaml"`
	Hash   string  `json:"hash"`
	Fields []Field `json:"fields"`
}

type Status struct {
	Protocol          int      `json:"protocol"`
	Privileged        bool     `json:"privileged"`
	Running           bool     `json:"running"`
	PID               int      `json:"pid"`
	KernelVersion     string   `json:"kernelVersion"`
	KernelPath        string   `json:"kernelPath"`
	ConfigPath        string   `json:"configPath"`
	ActiveRevision    string   `json:"activeRevision"`
	ActiveProfile     string   `json:"activeProfile"`
	Error             string   `json:"error,omitempty"`
	KernelDirectory   string   `json:"kernelDirectory"`
	RuntimeDirectory  string   `json:"runtimeDirectory"`
	KernelAuthorized  bool     `json:"kernelAuthorized"`
	ActiveTUN         bool     `json:"activeTun"`
	AuthorizedKernels []string `json:"authorizedKernels,omitempty"`
	GlobalOverrides   bool     `json:"globalOverrides,omitempty"`
}

type Snapshot struct {
	State   State    `json:"state"`
	Status  Status   `json:"status"`
	Kernels []Kernel `json:"kernels"`
}

type Log struct {
	Sequence uint64    `json:"sequence"`
	Time     time.Time `json:"time"`
	Level    string    `json:"level"`
	Source   string    `json:"source"`
	Message  string    `json:"message"`
}

type Connection struct {
	ID       string `json:"id"`
	Metadata struct {
		Network         string `json:"network"`
		Type            string `json:"type"`
		Host            string `json:"host"`
		DestinationIP   string `json:"destinationIP"`
		DestinationPort string `json:"destinationPort"`
		Process         string `json:"process"`
		ProcessPath     string `json:"processPath"`
	} `json:"metadata"`
	Upload      int64    `json:"upload"`
	Download    int64    `json:"download"`
	Start       string   `json:"start"`
	Chains      []string `json:"chains"`
	Rule        string   `json:"rule"`
	RulePayload string   `json:"rulePayload"`
}

// Traffic holds the running core's latest byte rates and cumulative counts.
type Traffic struct {
	Available       bool  `json:"available"`
	Up              int64 `json:"up"`
	Down            int64 `json:"down"`
	TotalsAvailable bool  `json:"totalsAvailable"`
	UpTotal         int64 `json:"upTotal"`
	DownTotal       int64 `json:"downTotal"`
}

const TrafficHistoryWindow = 5 * time.Minute

type TrafficPoint struct {
	At          time.Time `json:"at"`
	Up          int64     `json:"up"`
	Down        int64     `json:"down"`
	BreakBefore bool      `json:"breakBefore,omitempty"`
}

type TrafficSnapshot struct {
	Traffic
	History []TrafficPoint `json:"history,omitempty"`
}

type Connections struct {
	Connections   []Connection `json:"connections"`
	UploadTotal   int64        `json:"uploadTotal"`
	DownloadTotal int64        `json:"downloadTotal"`
}

type Proxy struct {
	Name string   `json:"name"`
	Type string   `json:"type"`
	Now  string   `json:"now"`
	All  []string `json:"all"`
}

type Release struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	AssetID int64  `json:"assetId"`
	Size    int64  `json:"size"`
	Digest  string `json:"digest,omitempty"`
	URL     string `json:"-"`
}

package ng

type PartConfig struct {
	Enabled   bool   `json:"enabled"`
	Position  int    `json:"position"`
	Separator string `json:"separator,omitempty"`
	Width     int    `json:"width,omitempty"`
	Length    int    `json:"length,omitempty"`
}

func (p *PartConfig) IsEnabled() bool { return p != nil && p.Enabled }

type PartConfigWithValue struct {
	PartConfig
	Value string
}

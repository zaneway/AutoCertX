package buildinfo

// These values are expected to be overridden at build time with -ldflags.
var (
	Version   = "dev"
	Commit    = "none"
	BuildTime = "unknown"
)

type Info struct {
	Service   string `json:"service"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
}

func Current(service string) Info {
	return Info{
		Service:   service,
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
	}
}

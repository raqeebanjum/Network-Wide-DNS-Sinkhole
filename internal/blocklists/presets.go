package blocklists

var defaultPresets = []Preset{
	{
		Slug:        "stevenblack",
		Name:        "StevenBlack Unified Hosts",
		URL:         "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts",
		License:     "MIT",
		Homepage:    "https://github.com/StevenBlack/hosts",
		Description: "Balanced ads, trackers, and malware domain coverage. Pi-hole's traditional default.",
	},
	{
		Slug:        "oisd-big",
		Name:        "OISD Big",
		URL:         "https://big.oisd.nl/",
		License:     "GPL-3.0",
		Homepage:    "https://oisd.nl/",
		Description: "Broad ads, tracking, malware, and app-ad coverage with low false-positive risk.",
	},
	{
		Slug:        "hagezi-pro",
		Name:        "HaGeZi Multi PRO",
		URL:         "https://cdn.jsdelivr.net/gh/hagezi/dns-blocklists@latest/adblock/pro.txt",
		License:     "GPL-3.0",
		Homepage:    "https://github.com/hagezi/dns-blocklists",
		Description: "Wider ads, tracking, telemetry, phishing, and malware coverage for managed networks.",
	},
}

// Presets returns a copy of the curated source catalog.
func Presets() []Preset {
	return append([]Preset(nil), defaultPresets...)
}

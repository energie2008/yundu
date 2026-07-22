package machine

const (
	XrayAPIPortRangeStart      = 10085
	XrayAPIPortRangeEnd        = 10584
	SingboxClashPortRangeStart = 20086
	SingboxClashPortRangeEnd   = 20585
)

func IsInternalAPIPort(port int) bool {
	return (port >= XrayAPIPortRangeStart && port <= XrayAPIPortRangeEnd) ||
		(port >= SingboxClashPortRangeStart && port <= SingboxClashPortRangeEnd)
}

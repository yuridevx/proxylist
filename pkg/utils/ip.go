package utils

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

func CleanHostPort(hostPort string) (string, int, error) {
	// Attempt to split the input into host and port
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		// If there's an error, assume the input is just a host with no port
		host = hostPort
		port = "0" // Default port value if none is provided
	}

	// Validate and clean the host part
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return "", 0, fmt.Errorf("invalid IP format")
	}

	for i, part := range parts {
		num, err := strconv.Atoi(part)
		if err != nil {
			return "", 0, fmt.Errorf("invalid IP segment: %s", part)
		}
		parts[i] = strconv.Itoa(num)
	}

	cleanedHost := strings.Join(parts, ".")
	if net.ParseIP(cleanedHost) == nil {
		return "", 0, fmt.Errorf("invalid IP address")
	}

	// Validate the port part
	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 0 || portNum > 65535 {
		return "", 0, fmt.Errorf("invalid port number")
	}

	return cleanedHost, portNum, nil
}

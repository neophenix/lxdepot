package shared

import (
	"fmt"
	"strconv"
	"strings"
)

type ContainerAction string

const (
	Stop     ContainerAction = "stop"
	Start    ContainerAction = "start"
	Restart  ContainerAction = "restart"
	Freeze   ContainerAction = "freeze"
	Unfreeze ContainerAction = "unfreeze"
)

func IsInt64(value string) error {
	if value == "" {
		return nil
	}

	_, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fmt.Errorf("Invalid value for an integer: %s", value)
	}

	return nil
}

func IsUint32(value string) error {
	if value == "" {
		return nil
	}

	_, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return fmt.Errorf("Invalid value for uint32: %s: %v", value, err)
	}

	return nil
}

func IsPriority(value string) error {
	if value == "" {
		return nil
	}

	valueInt, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fmt.Errorf("Invalid value for an integer: %s", value)
	}

	if valueInt < 0 || valueInt > 10 {
		return fmt.Errorf("Invalid value for a limit '%s'. Must be between 0 and 10.", value)
	}

	return nil
}

func IsBool(value string) error {
	if value == "" {
		return nil
	}

	if !StringInSlice(strings.ToLower(value), []string{"true", "false", "yes", "no", "1", "0", "on", "off"}) {
		return fmt.Errorf("Invalid value for a boolean: %s", value)
	}

	return nil
}

func IsOneOf(value string, valid []string) error {
	if value == "" {
		return nil
	}

	if !StringInSlice(value, valid) {
		return fmt.Errorf("Invalid value: %s (not one of %s)", value, valid)
	}

	return nil
}

func IsAny(value string) error {
	return nil
}

// IsRootDiskDevice returns true if the given device representation is
// configured as root disk for a container. It typically get passed a specific
// entry of api.Container.Devices.
func IsRootDiskDevice(device map[string]string) bool {
	if device["type"] == "disk" && device["path"] == "/" && device["source"] == "" {
		return true
	}

	return false
}

// GetRootDiskDevice returns the container device that is configured as root disk
func GetRootDiskDevice(devices map[string]map[string]string) (string, map[string]string, error) {
	var devName string
	var dev map[string]string

	for n, d := range devices {
		if IsRootDiskDevice(d) {
			if devName != "" {
				return "", nil, fmt.Errorf("More than one root device found.")
			}

			devName = n
			dev = d
		}
	}

	if devName != "" {
		return devName, dev, nil
	}

	return "", nil, fmt.Errorf("No root device could be found.")
}

// KnownContainerConfigKeys maps all fully defined, well-known config keys
// to an appropriate checker function, which validates whether or not a
// given value is syntactically legal.
var KnownContainerConfigKeys = map[string]func(value string) error{
	"boot.autostart":             IsBool,
	"boot.autostart.delay":       IsInt64,
	"boot.autostart.priority":    IsInt64,
	"boot.stop.priority":         IsInt64,
	"boot.host_shutdown_timeout": IsInt64,

	"limits.cpu": IsAny,
	"limits.cpu.allowance": func(value string) error {
		if value == "" {
			return nil
		}

		if strings.HasSuffix(value, "%") {
			// Percentage based allocation
			_, err := strconv.Atoi(strings.TrimSuffix(value, "%"))
			if err != nil {
				return err
			}

			return nil
		}

		// Time based allocation
		fields := strings.SplitN(value, "/", 2)
		if len(fields) != 2 {
			return fmt.Errorf("Invalid allowance: %s", value)
		}

		_, err := strconv.Atoi(strings.TrimSuffix(fields[0], "ms"))
		if err != nil {
			return err
		}

		_, err = strconv.Atoi(strings.TrimSuffix(fields[1], "ms"))
		if err != nil {
			return err
		}

		return nil
	},
	"limits.cpu.priority": IsPriority,

	"limits.disk.priority": IsPriority,

	"limits.memory": func(value string) error {
		if value == "" {
			return nil
		}

		if strings.HasSuffix(value, "%") {
			_, err := strconv.ParseInt(strings.TrimSuffix(value, "%"), 10, 64)
			if err != nil {
				return err
			}

			return nil
		}

		_, err := ParseByteSizeString(value)
		if err != nil {
			return err
		}

		return nil
	},
	"limits.memory.enforce": func(value string) error {
		return IsOneOf(value, []string{"soft", "hard"})
	},
	"limits.memory.swap":          IsBool,
	"limits.memory.swap.priority": IsPriority,

	"limits.network.priority": IsPriority,

	"limits.processes": IsInt64,

	"linux.kernel_modules": IsAny,

	"migration.incremental.memory":            IsBool,
	"migration.incremental.memory.iterations": IsUint32,
	"migration.incremental.memory.goal":       IsUint32,

	"nvidia.runtime": IsBool,

	"security.nesting":       IsBool,
	"security.privileged":    IsBool,
	"security.devlxd":        IsBool,
	"security.devlxd.images": IsBool,

	"security.protection.delete": IsBool,

	"security.idmap.base":     IsUint32,
	"security.idmap.isolated": IsBool,
	"security.idmap.size":     IsUint32,

	"security.syscalls.blacklist_default": IsBool,
	"security.syscalls.blacklist_compat":  IsBool,
	"security.syscalls.blacklist":         IsAny,
	"security.syscalls.whitelist":         IsAny,

	// Caller is responsible for full validation of any raw.* value
	"raw.apparmor": IsAny,
	"raw.lxc":      IsAny,
	"raw.seccomp":  IsAny,
	"raw.idmap":    IsAny,

	"volatile.apply_template":   IsAny,
	"volatile.base_image":       IsAny,
	"volatile.last_state.idmap": IsAny,
	"volatile.last_state.power": IsAny,
	"volatile.idmap.next":       IsAny,
	"volatile.idmap.base":       IsAny,
	"volatile.apply_quota":      IsAny,
}

// ConfigKeyChecker returns a function that will check whether or not
// a provide value is valid for the associate config key.  Returns an
// error if the key is not known.  The checker function only performs
// syntactic checking of the value, semantic and usage checking must
// be done by the caller.  User defined keys are always considered to
// be valid, e.g. user.* and environment.* keys.
func ConfigKeyChecker(key string) (func(value string) error, error) {
	if f, ok := KnownContainerConfigKeys[key]; ok {
		return f, nil
	}

	if strings.HasPrefix(key, "volatile.") {
		if strings.HasSuffix(key, ".hwaddr") {
			return IsAny, nil
		}

		if strings.HasSuffix(key, ".name") {
			return IsAny, nil
		}

		if strings.HasSuffix(key, ".host_name") {
			return IsAny, nil
		}
	}

	if strings.HasPrefix(key, "environment.") {
		return IsAny, nil
	}

	if strings.HasPrefix(key, "user.") {
		return IsAny, nil
	}

	if strings.HasPrefix(key, "image.") {
		return IsAny, nil
	}

	if strings.HasPrefix(key, "limits.kernel.") &&
		(len(key) > len("limits.kernel.")) {
		return IsAny, nil
	}

	return nil, fmt.Errorf("Unknown configuration key: %s", key)
}

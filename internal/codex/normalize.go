package codex

func normalizeWindows(primary, secondary *Window) (*Window, *Window) {
	switch {
	case primary != nil && secondary != nil:
		switch {
		case windowRole(*primary) == roleWeekly && windowRole(*secondary) != roleWeekly:
			return secondary, primary
		default:
			return primary, secondary
		}
	case primary != nil:
		if windowRole(*primary) == roleWeekly {
			return nil, primary
		}
		return primary, nil
	case secondary != nil:
		if windowRole(*secondary) == roleWeekly {
			return nil, secondary
		}
		return secondary, nil
	default:
		return nil, nil
	}
}

type role int

const (
	roleUnknown role = iota
	roleSession
	roleWeekly
)

func windowRole(window Window) role {
	if window.WindowMinutes == nil {
		return roleUnknown
	}
	switch *window.WindowMinutes {
	case 300:
		return roleSession
	case 10080:
		return roleWeekly
	default:
		return roleUnknown
	}
}

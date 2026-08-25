package hazard

func blocksIncidentClosure(plans, deployments int) bool {
	if plans > 0 {
		return true
	}
	if deployments > 0 {
		return true
	}
	return false
}

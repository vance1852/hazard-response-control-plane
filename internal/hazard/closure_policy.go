package hazard

func blocksIncidentClosure(plans, deployments int) bool {
	if plans > 0 {
		return true
	}
	return deployments > 0 && plans > 0
}

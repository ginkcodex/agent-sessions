package main

func installerArgs(operation, target, logPath string) []string {
	return []string{operation, target, "/quiet", "/norestart", "/L*v", logPath,
		"REBOOT=ReallySuppress", "MSIRESTARTMANAGERCONTROL=DisableShutdown", "MSIDISABLERMRESTART=1"}
}

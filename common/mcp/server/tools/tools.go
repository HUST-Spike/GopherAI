package tools

import (
	mcpserver "GopherAI/common/mcp/server"
)

// RegisterAll installs every built-in tool into the given registry.
//
// Adding a new tool requires exactly two edits:
//  1. Create a tools/<name>.go file with a registerXxx(reg) function.
//  2. Append registerXxx(reg) to the list below.
//
// Skill-restricted tools (e.g. run_python) are added in later steps; they
// follow the same pattern but pass a non-default ToolMeta when calling
// reg.Register.
func RegisterAll(reg *mcpserver.ToolRegistry) {
	registerWeather(reg)
	registerCurrentTime(reg)
	registerCalculator(reg)
	registerIPInfo(reg)
	registerGitHubRepo(reg)
	registerGitHubUser(reg)
	registerPyPIPackage(reg)
	registerNPMPackage(reg)
	registerCurrencyConvert(reg)
	registerRandomQuote(reg)
	registerDictionaryLookup(reg)
	registerFetchURLText(reg)

	registerSearchSessionDocuments(reg)
	registerListMyDocuments(reg)
	registerWebSearch(reg)

	registerRunPython(reg)
}

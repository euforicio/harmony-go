package harmony

import "encoding/json"

// BrowserToolNamespace returns the built-in browser namespace used by official Harmony prompts.
func BrowserToolNamespace() ToolNamespaceConfig {
	exampleOne := "`\u30106\u2020L9-L11\u3011`"
	exampleTwo := "`\u30108\u2020L3\u3011`"
	desc := "Tool for browsing.\n" +
		"The `cursor` appears in brackets before each browsing display: `[{cursor}]`.\n" +
		"Cite information from the tool using the following format:\n" +
		"`【{cursor}†L{line_start}(-L{line_end})?】`, for example: " + exampleOne + " or " + exampleTwo + ".\n" +
		"Do not quote more than 10 words directly from the tool output.\n" +
		"sources=web (default: web)"
	return ToolNamespaceConfig{
		Name:        "browser",
		Description: &desc,
		Tools: []ToolDescription{
			{
				Name:        "search",
				Description: "Searches for information related to `query` and displays `topn` results.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"topn":{"type":"number","default":10},"source":{"type":"string"}},"required":["query"]}`),
			},
			{
				Name:        "open",
				Description: "Opens the link `id` from the page indicated by `cursor` starting at line number `loc`, showing `num_lines` lines.\nValid link ids are displayed with the formatting: `【{id}†.*】`.\nIf `cursor` is not provided, the most recent page is implied.\nIf `id` is a string, it is treated as a fully qualified URL associated with `source`.\nIf `loc` is not provided, the viewport will be positioned at the beginning of the document or centered on the most relevant passage, if available.\nUse this function without `id` to scroll to a new location of an opened page.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"id":{"type":["number","string"],"default":-1},"cursor":{"type":"number","default":-1},"loc":{"type":"number","default":-1},"num_lines":{"type":"number","default":-1},"view_source":{"type":"boolean","default":false},"source":{"type":"string"}}}`),
			},
			{
				Name:        "find",
				Description: "Finds exact matches of `pattern` in the current page, or the page given by `cursor`.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"},"cursor":{"type":"number","default":-1}},"required":["pattern"]}`),
			},
		},
	}
}

// PythonToolNamespace returns the built-in python namespace used by official Harmony prompts.
func PythonToolNamespace() ToolNamespaceConfig {
	desc := "Use this tool to execute Python code in your chain of thought. The code will not be shown to the user. This tool should be used for internal reasoning, but not for code that is intended to be visible to the user (e.g. when creating plots, tables, or files).\n\nWhen you send a message containing Python code to python, it will be executed in a stateful Jupyter notebook environment. python will respond with the output of the execution or time out after 120.0 seconds. The drive at '/mnt/data' can be used to save and persist user files. Internet access for this session is UNKNOWN. Depends on the cluster."
	return ToolNamespaceConfig{
		Name:        "python",
		Description: &desc,
	}
}

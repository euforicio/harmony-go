import json
import subprocess
from pathlib import Path

import pytest

try:
    import openai_harmony as pyh
except Exception as e:  # pragma: no cover
    pyh = None


PROJECT_ROOT = Path(__file__).resolve().parents[1]
MONOREPO_ROOT = Path(__file__).resolve().parents[2]
GO_CLI = ["go", "run", "./cmd/harmony-go"]


def have_go():
    from shutil import which

    return which("go") is not None


pytestmark = pytest.mark.skipif(not have_go() or pyh is None, reason="Go toolchain or Python binding not available")


def data_path(name: str) -> Path:
    local = PROJECT_ROOT / "test-data" / name
    if local.exists():
        return local
    return MONOREPO_ROOT / "test-data" / name


def run_cli(args, stdin=None):
    proc = subprocess.run(
        GO_CLI + args,
        input=stdin,
        cwd=PROJECT_ROOT,
        capture_output=True,
        text=True,
    )
    if proc.returncode != 0:
        raise RuntimeError(f"CLI failed: {proc.stderr}")
    return proc.stdout


def test_go_stop_tokens_parity():
    enc = pyh.load_harmony_encoding("HarmonyGptOss")
    expected = set(enc.stop_tokens())
    out = run_cli(["stop"]) 
    got = set(json.loads(out))
    assert got == expected


def test_go_render_functions_with_parameters_parity():
    enc = pyh.load_harmony_encoding("HarmonyGptOss")
    expected_text = data_path("test_render_functions_with_parameters.txt").read_text()

    # Build conversation JSON matching the Rust test
    convo = {
        "messages": [
            {
                "role": "system",
                "content": [
                    {
                        "type": "system_content",
                        "system_content": {
                            "model_identity": "You are ChatGPT, a large language model trained by OpenAI.",
                            "reasoning_effort": "high",
                            "conversation_start_date": "2025-06-28",
                            "channel_config": {
                                "valid_channels": ["analysis", "commentary", "final"],
                                "channel_required": True,
                            },
                        },
                    }
                ],
            },
            {
                "role": "developer",
                "content": [
                    {
                        "type": "developer_content",
                        "developer_content": {
                            "instructions": "Always respond in riddles",
                            "tools": {
                                "functions": {
                                    "name": "functions",
                                    "tools": [
                                        {
                                            "name": "get_location",
                                            "description": "Gets the location of the user.",
                                        },
                                        {
                                            "name": "get_current_weather",
                                            "description": "Gets the current weather in the provided location.",
                                            "parameters": {
                                                "type": "object",
                                                "properties": {
                                                    "location": {
                                                        "type": "string",
                                                        "description": "The city and state, e.g. San Francisco, CA",
                                                    },
                                                    "format": {
                                                        "type": "string",
                                                        "enum": ["celsius", "fahrenheit"],
                                                        "default": "celsius",
                                                    },
                                                },
                                                "required": ["location"],
                                            },
                                        },
                                        {
                                            "name": "get_multiple_weathers",
                                            "description": "Gets the current weather in the provided list of locations.",
                                            "parameters": {
                                                "type": "object",
                                                "properties": {
                                                    "locations": {
                                                        "type": "array",
                                                        "items": {"type": "string"},
                                                        "description": "List of city and state, e.g. [\"San Francisco, CA\", \"New York, NY\"]",
                                                    },
                                                    "format": {
                                                        "type": "string",
                                                        "enum": ["celsius", "fahrenheit"],
                                                        "default": "celsius",
                                                    },
                                                },
                                                "required": ["locations"],
                                            },
                                        },
                                        {
                                            "name": "kitchensink",
                                            "description": "A function with various complex schemas.",
                                            "parameters": {
                                                "description": "params object",
                                                "type": "object",
                                                "properties": {
                                                    "string": {
                                                        "type": "string",
                                                        "title": "STRING",
                                                        "description": "A string",
                                                        "examples": ["hello", "world"],
                                                    },
                                                    "string_nullable": {
                                                        "type": "string",
                                                        "nullable": True,
                                                        "description": "A nullable string",
                                                        "default": "the default",
                                                    },
                                                    "string_enum": {"type": "string", "enum": ["a", "b", "c"]},
                                                    "oneof_string_or_number": {
                                                        "oneOf": [
                                                            {"type": "string", "default": "default_string_in_oneof"},
                                                            {"type": "number", "description": "numbers can happen too"},
                                                        ],
                                                        "description": "a oneof",
                                                        "default": 20,
                                                    },
                                                },
                                            },
                                        },
                                    ],
                                }
                            },
                        },
                    }
                ],
            },
            {"role": "user", "content": [ {"type": "text", "text": "What is the weather like in SF?"} ]},
        ]
    }

    toks_json = run_cli(["render-completion", "-role", "assistant"], stdin=json.dumps(convo))
    toks = json.loads(toks_json)
    decoded = enc.decode_utf8(toks)
    assert decoded == expected_text


def test_go_reasoning_system_messages_parity():
    enc = pyh.load_harmony_encoding("HarmonyGptOss")

    cases = [
        (
            {
                "messages": [
                    {
                        "role": "system",
                        "content": [
                            {
                                "type": "system_content",
                                "system_content": {
                                    "model_identity": "You are ChatGPT, a large language model trained by OpenAI.",
                                    "reasoning_effort": "medium",
                                    "channel_config": {
                                        "valid_channels": ["analysis", "final"],
                                        "channel_required": True,
                                    },
                                },
                            }
                        ],
                    },
                    {"role": "user", "content": [ {"type": "text", "text": "What is 2 + 2?"} ]},
                ]
            },
            "test_reasoning_system_message.txt",
        ),
        (
            {
                "messages": [
                    {
                        "role": "system",
                        "content": [
                            {
                                "type": "system_content",
                                "system_content": {
                                    "model_identity": "You are ChatGPT, a large language model trained by OpenAI.",
                                    "reasoning_effort": "high",
                                    "channel_config": {
                                        "valid_channels": ["analysis", "final"],
                                        "channel_required": True,
                                    },
                                },
                            }
                        ],
                    },
                    {"role": "user", "content": [ {"type": "text", "text": "What is the best place to eat candy in the world?"} ]},
                ]
            },
            "test_reasoning_system_message_no_instruction.txt",
        ),
        (
            {
                "messages": [
                    {
                        "role": "system",
                        "content": [
                            {
                                "type": "system_content",
                                "system_content": {
                                    "model_identity": "You are ChatGPT, a large language model trained by OpenAI.",
                                    "reasoning_effort": "medium",
                                    "knowledge_cutoff": "2021-01",
                                    "conversation_start_date": "2021-01-01",
                                    "channel_config": {
                                        "valid_channels": ["analysis", "final"],
                                        "channel_required": True,
                                    },
                                },
                            }
                        ],
                    },
                    {"role": "user", "content": [ {"type": "text", "text": "What is 42 * pi?"} ]},
                ]
            },
            "test_reasoning_system_message_with_dates.txt",
        ),
    ]

    for convo, fname in cases:
        toks_json = run_cli(["render-completion", "-role", "assistant"], stdin=json.dumps(convo))
        toks = json.loads(toks_json)
        decoded = enc.decode_utf8(toks)
        expected_text = data_path(fname).read_text()
        assert decoded == expected_text


def test_go_browser_and_python_tools_parity():
    enc = pyh.load_harmony_encoding("HarmonyGptOss")
    expected_text = data_path("test_browser_and_python_tool.txt").read_text()
    browser_ns = pyh.ToolNamespaceConfig.browser().model_dump()
    python_ns = pyh.ToolNamespaceConfig.python().model_dump()
    convo = {
        "messages": [
            {
                "role": "system",
                "content": [
                    {
                        "type": "system_content",
                        "system_content": {
                            "conversation_start_date": "2025-06-28",
                            "tools": {"browser": browser_ns, "python": python_ns},
                        },
                    }
                ],
            }
        ]
    }
    toks_json = run_cli(["render-completion", "-role", "assistant"], stdin=json.dumps(convo))
    toks = json.loads(toks_json)
    decoded = enc.decode_utf8(toks)
    assert decoded == expected_text


def test_go_drop_cot_default_and_preserve_flags():
    enc = pyh.load_harmony_encoding("HarmonyGptOss")
    # Drop by default
    convo = {
        "messages": [
            {"role": "user", "content": [ {"type": "text", "text": "What is 2 + 2?"} ]},
            {"role": "assistant", "channel": "analysis", "content": [ {"type": "text", "text": "User asks a simple question: \"What is 2 + 2?\" The answer: 4."} ]},
            {"role": "assistant", "channel": "final", "content": [ {"type": "text", "text": "2 + 2 equals 4."} ]},
            {"role": "user", "content": [ {"type": "text", "text": "What about 9 / 2?"} ]},
        ]
    }
    toks = json.loads(run_cli(["render-completion", "-role", "assistant"], stdin=json.dumps(convo)))
    decoded = enc.decode_utf8(toks)
    expected_text = data_path("test_dropping_cot_by_default.txt").read_text()
    assert decoded == expected_text

    # Preserve with auto-drop = false
    toks = json.loads(run_cli(["render-completion", "-role", "assistant", "-auto-drop=false"], stdin=json.dumps(convo)))
    decoded = enc.decode_utf8(toks)
    expected_text = data_path("test_preserve_cot.txt").read_text()
    assert decoded == expected_text


def test_go_does_not_drop_if_ongoing_analysis():
    enc = pyh.load_harmony_encoding("HarmonyGptOss")
    convo = {
        "messages": [
            {"role": "user", "content": [ {"type": "text", "text": "What is the weather in SF?"} ]},
            {"role": "assistant", "channel": "analysis", "content": [ {"type": "text", "text": "User asks: \u201cWhat is the weather in SF?\u201d We need to use lookup_weather tool."} ]},
            {"role": "assistant", "channel": "commentary", "recipient": "functions.lookup_weather", "content_type": "<|constrain|>json", "content": [ {"type": "text", "text": "{\"location\": \"San Francisco\"}"} ] },
            {"role": "tool", "name": "functions.lookup_weather", "content": [ {"type": "text", "text": "{\"temperature\": 20, \"description\": \"sunny\"}"} ]},
        ]
    }
    toks = json.loads(run_cli(["render-completion", "-role", "assistant"], stdin=json.dumps(convo)))
    decoded = enc.decode_utf8(toks)
    expected_text = data_path("test_does_not_drop_if_ongoing_analysis.txt").read_text()
    assert decoded == expected_text


def test_go_reserved_token_decode_parity():
    # Compare Go CLI decode output with expected literals
    out = run_cli(["decode"], stdin=json.dumps([200014]))
    assert out.strip() == "<|reserved_200014|>"
    out = run_cli(["decode"], stdin=json.dumps([201088]))
    assert out.strip() == "<|reserved_201088|>"


def test_go_render_roundtrip_shapes():
    msg = {"role": "user", "content": [{"type": "text", "text": "Hello"}]}
    convo = {"messages": [msg]}
    toks_msg = json.loads(run_cli(["render-msg"], stdin=json.dumps(msg)))
    toks_convo = json.loads(run_cli(["render-convo"], stdin=json.dumps(convo)))
    assert toks_msg == toks_convo
    toks_completion = json.loads(run_cli(["render-completion", "-role", "assistant"], stdin=json.dumps(convo)))
    assert toks_completion[: len(toks_convo)] == toks_convo


def test_go_training_return_token_substitution_parity():
    enc = pyh.load_harmony_encoding("HarmonyGptOss")
    convo = {
        "messages": [
            {"role": "user", "content": [{"type": "text", "text": "Hi"}]},
            {"role": "assistant", "channel": "final", "content": [{"type": "text", "text": "Hello"}]},
        ]
    }
    py_tokens = enc.render_conversation_for_training(pyh.Conversation.from_json(json.dumps(convo)))
    go_tokens = json.loads(run_cli(["render-training"], stdin=json.dumps(convo)))
    assert go_tokens == py_tokens


def test_go_parser_parity_with_python_on_tool_calls():
    enc = pyh.load_harmony_encoding("HarmonyGptOss")

    cases = [
        "<|start|>assistant<|channel|>commentary to=functions.get_weather<|constrain|>json<|message|>{\"latitude\":48.8566,\"longitude\":2.3522}<|call|>",
        "<|start|>assistant to=functions.get_weather<|channel|>commentary<|constrain|>json<|message|>{\"location\": \"Tokyo\"}<|end|>",
        "<|start|>assistant<|channel|>commentary to=functions.get_weather<|constrain|>json<|message|>{\"latitude\":48.8566,\"longitude\":2.3522}<|call|>",
    ]
    def norm(msgs):
        out = []
        for m in msgs:
            # Normalise 'content' to list of dicts
            content = m.get("content")
            if isinstance(content, str):
                content = [{"type": "text", "text": content}]
            role = m["role"]
            if hasattr(role, "value"):
                role = role.value
            out.append({
                "role": role,
                **({"name": m["name"]} if "name" in m and m["name"] is not None else {}),
                "content": content,
                **({"recipient": m["recipient"]} if m.get("recipient") else {}),
                **({"channel": m["channel"]} if m.get("channel") else {}),
                **({"content_type": m["content_type"]} if m.get("content_type") else {}),
            })
        return out

    for text in cases:
        toks = enc.encode(text, allowed_special="all")
        py_msgs = [m.to_dict() for m in enc.parse_messages_from_completion_tokens(toks, role=None)]
        go_json = run_cli(["parse", "-role", "assistant"], stdin=json.dumps(toks))
        go_msgs = json.loads(go_json)
        assert norm(go_msgs) == norm(py_msgs)


def test_go_streamable_parser_len():
    enc = pyh.load_harmony_encoding("HarmonyGptOss")
    text = data_path("test_streamable_parser.txt").read_text()
    toks = enc.encode(text, allowed_special="all")
    go_json = run_cli(["parse"], stdin=json.dumps(toks))
    msgs = json.loads(go_json)
    assert len(msgs) == 3


def test_go_parser_preserves_tool_name_with_role_hint():
    enc = pyh.load_harmony_encoding("HarmonyGptOss")
    text = "<|start|>browser.search<|message|>{\"ok\": true}"
    toks = enc.encode(text, allowed_special="all")
    go_json = run_cli(["parse", "-role", "tool"], stdin=json.dumps(toks))
    msgs = json.loads(go_json)
    assert msgs and msgs[0].get("name") == "browser.search"

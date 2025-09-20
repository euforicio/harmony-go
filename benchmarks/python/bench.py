#!/usr/bin/env python
import argparse
import json
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path


from openai_harmony import (
    Author,
    ChannelConfig,
    Conversation,
    DeveloperContent,
    HarmonyEncodingName,
    Message,
    RenderConversationConfig,
    Role,
    StreamableParser,
    SystemContent,
    ToolDescription,
    ToolNamespaceConfig,
    load_harmony_encoding,
)


PROJECT_ROOT = Path(__file__).resolve().parents[2]

TOOL_CALL_TEXT = (
    "<|start|>assistant<|channel|>commentary to=functions.get_weather"
    '<|constrain|>json<|message|>{"latitude":48.8566,"longitude":2.3522}<|call|>'
)


@dataclass
class BenchmarkInput:
    name: str
    conversation: Conversation | None = None
    config: RenderConversationConfig | None = None
    tokens: list[int] | None = None
    role: Role | None = None


def make_tool_conversation() -> Conversation:
    messages = [
        Message.from_role_and_content(Role.USER, "What is the weather in SF?"),
        Message.from_role_and_content(
            Role.ASSISTANT,
            "User asks: \u201cWhat is the weather in SF?\u201d We need to use lookup_weather tool.",
        ).with_channel("analysis"),
        Message.from_role_and_content(
            Role.ASSISTANT, '{"location": "San Francisco"}'
        )
        .with_channel("commentary")
        .with_recipient("functions.lookup_weather")
        .with_content_type("<|constrain|>json"),
        Message.from_author_and_content(
            Author.new(Role.TOOL, "functions.lookup_weather"),
            '{"temperature": 20, "description": "sunny"}',
        ),
    ]
    return Conversation.from_messages(messages)


def make_large_conversation() -> Conversation:
    big_block = (
        "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Vestibulum vulputate. "
        * 200
    )

    sys = (
        SystemContent.new()
        .with_model_identity("Test model")
        .with_knowledge_cutoff("2024-06")
        .with_conversation_start_date("2025-09-20")
        .with_channel_config(ChannelConfig.require_channels(["analysis", "commentary", "final"]))
        .with_browser_tool()
    )

    developer_tools = [
        ToolDescription.new(
            name="get_location",
            description="Gets the location of the user.",
        ),
        ToolDescription.new(
            name="get_current_weather",
            description="Gets the current weather in the provided location.",
            parameters={
                "type": "object",
                "properties": {
                    "location": {
                        "type": "string",
                        "description": "City and state, e.g. San Francisco, CA",
                    },
                    "format": {
                        "type": "string",
                        "enum": ["celsius", "fahrenheit"],
                        "default": "celsius",
                    },
                },
                "required": ["location"],
            },
        ),
    ]

    dev = (
        DeveloperContent.new()
        .with_instructions("Follow tool schema precisely. " * 100)
        .with_function_tools(developer_tools)
        .with_tools(ToolNamespaceConfig.python())
    )

    messages = [
        Message.from_role_and_content(Role.SYSTEM, sys),
        Message.from_role_and_content(Role.DEVELOPER, dev).with_channel("analysis"),
    ]

    for idx in range(8):
        messages.append(
            Message.from_role_and_content(
                Role.USER,
                f"User block {idx}: {big_block}",
            )
        )
        messages.append(
            Message.from_role_and_content(
                Role.ASSISTANT,
                f"Assistant analysis {idx}: {big_block}",
            ).with_channel("analysis")
        )

    messages.append(
        Message.from_role_and_content(
            Role.ASSISTANT,
            "Final answer summarizing the conversation.",
        ).with_channel("final"),
    )

    return Conversation.from_messages(messages)


def make_large_completion_tokens(enc) -> list[int]:
    big_block = (
        "Reasoning chunk consolidating evidence. "
        * 120
    )
    analysis_msg = Message.from_role_and_content(
        Role.ASSISTANT,
        f"Chain-of-thought summary start. {big_block}",
    ).with_channel("analysis")

    final_msg = Message.from_role_and_content(
        Role.ASSISTANT,
        "Final answer summarizing salient points." + " " + big_block[: len(big_block) // 2],
    ).with_channel("final")

    completion = Conversation.from_messages([analysis_msg, final_msg])
    return enc.render_conversation(
        completion, RenderConversationConfig(auto_drop_analysis=False)
    )


def bench_python_render(enc, bench: BenchmarkInput, iters: int) -> dict:
    assert bench.conversation is not None
    config = bench.config or RenderConversationConfig()
    _ = enc.render_conversation_for_completion(
        bench.conversation, Role.ASSISTANT, config
    )
    t0 = time.perf_counter()
    for _ in range(iters):
        enc.render_conversation_for_completion(
            bench.conversation, Role.ASSISTANT, config
        )
    dt = time.perf_counter() - t0
    ns = (dt / iters) * 1e9
    return {
        "bench": bench.name,
        "iters": iters,
        "nanos_per_op": ns,
        "ops_per_sec": 1e9 / ns,
    }


def bench_python_parse(enc, bench: BenchmarkInput, iters: int) -> dict:
    assert bench.tokens is not None
    _ = enc.parse_messages_from_completion_tokens(bench.tokens, bench.role)
    t0 = time.perf_counter()
    for _ in range(iters):
        enc.parse_messages_from_completion_tokens(bench.tokens, bench.role)
    dt = time.perf_counter() - t0
    ns = (dt / iters) * 1e9
    return {
        "bench": bench.name,
        "iters": iters,
        "nanos_per_op": ns,
        "ops_per_sec": 1e9 / ns,
    }


def bench_python_stream_parser(enc, bench: BenchmarkInput, iters: int) -> dict:
    assert bench.tokens is not None
    t0 = time.perf_counter()
    for _ in range(iters):
        parser = StreamableParser(enc, bench.role)
        for token in bench.tokens:
            parser.process(token)
    dt = time.perf_counter() - t0
    ns = (dt / iters) * 1e9
    return {
        "bench": bench.name,
        "iters": iters,
        "nanos_per_op": ns,
        "ops_per_sec": 1e9 / ns,
    }


def bench_go(which: str, iters: int, benchmem: bool) -> list[dict]:
    pattern = {
        "render": "^BenchmarkRender",
        "parse": "^Benchmark(Parse|StreamParse)",
        "both": "^Benchmark",
    }[which]

    cmd = [
        "go",
        "test",
        "-json",
        "-run",
        "^$",
        "-bench",
        pattern,
        f"-benchtime={iters}x",
        "-count",
        "1",
    ]
    if benchmem:
        cmd.append("-benchmem")
    cmd.append("./benchmarks/go")

    proc = subprocess.run(
        cmd,
        cwd=PROJECT_ROOT,
        capture_output=True,
        text=True,
    )
    if proc.returncode != 0:
        raise RuntimeError(proc.stderr or proc.stdout)
    return parse_go_benchmarks(proc.stdout)


def parse_go_benchmarks(output: str) -> list[dict]:
    results: list[dict] = []
    for line in output.splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if event.get("Action") != "output":
            continue
        test_name = event.get("Test", "")
        if not test_name.startswith("Benchmark"):
            continue
        parsed = parse_go_benchmark_line(event.get("Output", ""))
        if parsed:
            results.append(parsed)
    return results


def parse_go_benchmark_line(line: str) -> dict | None:
    fields = line.strip().split()
    if len(fields) < 4:
        return None
    if not fields[0].startswith("Benchmark"):
        return None

    # Example layout:
    # BenchmarkRenderToolCall-12    1000    12345 ns/op    0 B/op    0 allocs/op
    bench_tag = fields[0].split("-")[0]
    bench_name = "go_" + camel_to_snake(bench_tag.removeprefix("Benchmark"))

    try:
        iters = int(fields[1])
        nanos_per_op = float(fields[2])
    except ValueError:
        return None

    result = {
        "bench": bench_name,
        "iters": iters,
        "nanos_per_op": nanos_per_op,
        "ops_per_sec": 1e9 / nanos_per_op if nanos_per_op else 0.0,
    }

    for idx, token in enumerate(fields):
        if token == "B/op" and idx:
            try:
                result["bytes_per_op"] = float(fields[idx - 1])
            except ValueError:
                pass
        if token == "allocs/op" and idx:
            try:
                result["allocs_per_op"] = float(fields[idx - 1])
            except ValueError:
                pass

    return result


def camel_to_snake(name: str) -> str:
    pieces: list[str] = []
    for idx, char in enumerate(name):
        if char.isupper() and idx != 0:
            pieces.append("_")
        pieces.append(char.lower())
    return "".join(pieces)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--iters", type=int, default=5000)
    ap.add_argument("--which", choices=["render", "parse", "both"], default="both")
    ap.add_argument("--benchmem", action="store_true", help="include Go allocation metrics")
    args = ap.parse_args()

    enc = load_harmony_encoding(HarmonyEncodingName.HARMONY_GPT_OSS)
    tool_convo = make_tool_conversation()
    large_convo = make_large_conversation()

    tool_tokens = enc.encode(TOOL_CALL_TEXT, allowed_special="all")
    large_completion_tokens = make_large_completion_tokens(enc)

    render_inputs = [
        BenchmarkInput(
            name="py_render_tool_call",
            conversation=tool_convo,
            config=RenderConversationConfig(auto_drop_analysis=True),
        ),
        BenchmarkInput(
            name="py_render_large_autodrop",
            conversation=large_convo,
            config=RenderConversationConfig(auto_drop_analysis=True),
        ),
        BenchmarkInput(
            name="py_render_large_keep_analysis",
            conversation=large_convo,
            config=RenderConversationConfig(auto_drop_analysis=False),
        ),
    ]

    parse_inputs = [
        BenchmarkInput(
            name="py_parse_tool_call",
            tokens=tool_tokens,
            role=None,
        ),
        BenchmarkInput(
            name="py_stream_parse_tool_call",
            tokens=tool_tokens,
            role=None,
        ),
        BenchmarkInput(
            name="py_parse_large_completion",
            tokens=large_completion_tokens,
            role=None,
        ),
        BenchmarkInput(
            name="py_stream_parse_large_completion",
            tokens=large_completion_tokens,
            role=None,
        ),
    ]

    out: list[dict] = []
    if args.which in ("render", "both"):
        for bench in render_inputs:
            out.append(bench_python_render(enc, bench, args.iters))
        out.extend(bench_go("render", args.iters, args.benchmem))
    if args.which in ("parse", "both"):
        # First two entries use parse, next two use streaming parser
        out.append(bench_python_parse(enc, parse_inputs[0], args.iters))
        out.append(bench_python_stream_parser(enc, parse_inputs[1], args.iters))
        out.append(bench_python_parse(enc, parse_inputs[2], args.iters))
        out.append(bench_python_stream_parser(enc, parse_inputs[3], args.iters))
        out.extend(bench_go("parse", args.iters, args.benchmem))

    print(json.dumps(out, indent=2))


if __name__ == "__main__":
    main()

use anyhow::Result;
use clap::Parser;
use serde::Serialize;
use std::time::Instant;

use openai_harmony::chat::{
    Author, ChannelConfig, Conversation, DeveloperContent, Message, Role, SystemContent,
    ToolDescription, ToolNamespaceConfig,
};
use openai_harmony::{load_harmony_encoding, HarmonyEncoding, HarmonyEncodingName, StreamableParser};

#[derive(Parser, Debug)]
#[command(name = "harmony_rust_bench", version, about = "Rust Harmony micro-benchmarks")]
struct Args {
    #[arg(long, default_value_t = 200)]
    iters: u64,
    #[arg(long, default_value = "both")]
    which: String,
}

#[derive(Serialize)]
struct BenchResult<'a> {
    bench: &'a str,
    iters: u64,
    nanos_per_op: f64,
    ops_per_sec: f64,
}

fn measure_ns_per_op<F: FnMut()>(iters: u64, mut f: F) -> f64 {
    // Warmup one run
    f();
    let start = Instant::now();
    for _ in 0..iters {
        f();
    }
    let dt = start.elapsed();
    (dt.as_secs_f64() / (iters as f64)) * 1e9
}

fn make_tool_conversation() -> Conversation {
    let mut messages = Vec::new();
    messages.push(Message::from_role_and_content(
        Role::User,
        "What is the weather in SF?",
    ));
    messages.push(
        Message::from_role_and_content(
            Role::Assistant,
            "User asks: \"What is the weather in SF?\" We need to use lookup_weather tool.",
        )
        .with_channel("analysis"),
    );
    messages.push(
        Message::from_role_and_content(Role::Assistant, "{\"location\": \"San Francisco\"}")
            .with_channel("commentary")
            .with_recipient("functions.lookup_weather")
            .with_content_type("<|constrain|>json"),
    );
    messages.push(Message::from_author_and_content(
        Author::new(Role::Tool, "functions.lookup_weather"),
        "{\"temperature\": 20, \"description\": \"sunny\"}",
    ));
    Conversation::from_messages(messages)
}

fn make_large_conversation() -> Conversation {
    let big_block = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Vestibulum vulputate. ".repeat(200);

    let sys = SystemContent::new()
        .with_model_identity("Test model")
        .with_knowledge_cutoff("2024-06")
        .with_conversation_start_date("2025-09-20")
        .with_channel_config(ChannelConfig::require_channels([
            "analysis", "commentary", "final",
        ]))
        .with_browser_tool();

    let developer_tools = vec![
        ToolDescription::new(
            "get_location",
            "Gets the location of the user.",
            None,
        ),
        ToolDescription::new(
            "get_current_weather",
            "Gets the current weather in the provided location.",
            Some(serde_json::json!({
                "type": "object",
                "properties": {
                    "location": {"type": "string", "description": "City and state, e.g. San Francisco, CA"},
                    "format": {"type": "string", "enum": ["celsius", "fahrenheit"], "default": "celsius"}
                },
                "required": ["location"]
            })),
        ),
    ];

    let dev = DeveloperContent::new()
        .with_instructions(&format!("{}", "Follow tool schema precisely. ".repeat(100)))
        .with_function_tools(developer_tools)
        .with_tools(ToolNamespaceConfig::python());

    let mut messages = Vec::new();
    messages.push(Message::from_role_and_content(Role::System, sys));
    messages.push(
        Message::from_role_and_content(Role::Developer, dev).with_channel("analysis"),
    );

    for idx in 0..8u8 {
        messages.push(Message::from_role_and_content(
            Role::User,
            format!("User block {}: {}", idx, big_block),
        ));
        messages.push(
            Message::from_role_and_content(
                Role::Assistant,
                format!("Assistant analysis {}: {}", idx, big_block),
            )
            .with_channel("analysis"),
        );
    }

    messages.push(
        Message::from_role_and_content(
            Role::Assistant,
            "Final answer summarizing the conversation.",
        )
        .with_channel("final"),
    );

    Conversation::from_messages(messages)
}

fn make_large_completion_tokens(enc: &HarmonyEncoding) -> Vec<u32> {
    let big_block = "Reasoning chunk consolidating evidence. ".repeat(120);
    let analysis_msg = Message::from_role_and_content(
        Role::Assistant,
        format!("Chain-of-thought summary start. {}", big_block),
    )
    .with_channel("analysis");
    let final_msg = Message::from_role_and_content(
        Role::Assistant,
        format!(
            "Final answer summarizing salient points. {}",
            &big_block[..big_block.len() / 2]
        ),
    )
    .with_channel("final");
    let completion = Conversation::from_messages(vec![analysis_msg, final_msg]);
    // Passing None for config means no auto-drop of analysis.
    enc.render_conversation(&completion, None).expect("render large completion")
}

fn main() -> Result<()> {
    let args = Args::parse();
    let enc: HarmonyEncoding =
        load_harmony_encoding(HarmonyEncodingName::HarmonyGptOss).expect("load encoding");

    let mut out: Vec<BenchResult> = Vec::new();

    if args.which == "render" || args.which == "both" {
        let tool_convo = make_tool_conversation();
        let large_convo = make_large_conversation();

        let ns = measure_ns_per_op(args.iters, || {
            let _ = enc
                .render_conversation_for_completion(&tool_convo, Role::Assistant, None)
                .expect("render tool convo");
        });
        out.push(BenchResult { bench: "rs_render_tool_call", iters: args.iters, nanos_per_op: ns, ops_per_sec: 1e9f64 / ns });

        // Note: RenderConversationConfig is not exported; we render without auto-drop (None).
        let ns = measure_ns_per_op(args.iters, || {
            let _ = enc
                .render_conversation_for_completion(&large_convo, Role::Assistant, None)
                .expect("render large convo");
        });
        out.push(BenchResult { bench: "rs_render_large_keep_analysis", iters: args.iters, nanos_per_op: ns, ops_per_sec: 1e9f64 / ns });
    }

    if args.which == "parse" || args.which == "both" {
        let tool_call_text = "<|start|>assistant<|channel|>commentary to=functions.get_weather<|constrain|>json<|message|>{\"latitude\":48.8566,\"longitude\":2.3522}<|call|>";
        let tool_tokens: Vec<u32> = enc
            .tokenizer()
            .encode_with_special_tokens(tool_call_text);
        let large_completion_tokens = make_large_completion_tokens(&enc);

        let ns = measure_ns_per_op(args.iters, || {
            let _ = enc
                .parse_messages_from_completion_tokens(tool_tokens.iter().copied(), None)
                .expect("parse tool tokens");
        });
        out.push(BenchResult { bench: "rs_parse_tool_call", iters: args.iters, nanos_per_op: ns, ops_per_sec: 1e9f64 / ns });

        let ns = measure_ns_per_op(args.iters, || {
            let mut p = StreamableParser::new(enc.clone(), None).expect("parser");
            for &t in &tool_tokens { p.process(t).expect("process"); }
            let _ = p.process_eos();
        });
        out.push(BenchResult { bench: "rs_stream_parse_tool_call", iters: args.iters, nanos_per_op: ns, ops_per_sec: 1e9f64 / ns });

        let ns = measure_ns_per_op(args.iters, || {
            let _ = enc
                .parse_messages_from_completion_tokens(large_completion_tokens.iter().copied(), None)
                .expect("parse large completion");
        });
        out.push(BenchResult { bench: "rs_parse_large_completion", iters: args.iters, nanos_per_op: ns, ops_per_sec: 1e9f64 / ns });

        let ns = measure_ns_per_op(args.iters, || {
            let mut p = StreamableParser::new(enc.clone(), None).expect("parser");
            for &t in &large_completion_tokens { p.process(t).expect("process"); }
            let _ = p.process_eos();
        });
        out.push(BenchResult { bench: "rs_stream_parse_large_completion", iters: args.iters, nanos_per_op: ns, ops_per_sec: 1e9f64 / ns });
    }

    println!("{}", serde_json::to_string_pretty(&out)?);
    Ok(())
}

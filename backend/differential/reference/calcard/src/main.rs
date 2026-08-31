use std::io::{self, Read, Write};
use std::process::ExitCode;

fn main() -> ExitCode {
    let args: Vec<String> = std::env::args().collect();
    if args.len() != 2 {
        eprintln!("usage: calcard-ref <vcard-to-jscontact|jscontact-to-vcard>");
        return ExitCode::from(2);
    }
    let mut input = String::new();
    if let Err(err) = io::stdin().read_to_string(&mut input) {
        eprintln!("calcard-ref: reading stdin: {err}");
        return ExitCode::from(1);
    }
    let result = match args[1].as_str() {
        "vcard-to-jscontact" => vcard_to_jscontact(&input),
        "jscontact-to-vcard" => jscontact_to_vcard(&input),
        "jscontact-reemit" => jscontact_reemit(&input),
        other => {
            eprintln!("calcard-ref: unknown subcommand {other:?}");
            return ExitCode::from(2);
        }
    };
    match result {
        Ok(out) => {
            let _ = io::stdout().write_all(out.as_bytes());
            ExitCode::SUCCESS
        }
        Err(err) => {
            eprintln!("calcard-ref: {err}");
            ExitCode::from(1)
        }
    }
}

fn vcard_to_jscontact(input: &str) -> Result<String, String> {
    let vcard = calcard::vcard::VCard::parse(input)
        .map_err(|e| format!("vCard parse failed: {e:?}"))?;
    let jscontact: calcard::jscontact::JSContact<'static, String, String> =
        vcard.into_jscontact::<String, String>();
    Ok(serde_json::to_string_pretty(&jscontact.0).map_err(|e| format!("JSContact serialize failed: {e}"))?)
}

fn jscontact_reemit(input: &str) -> Result<String, String> {
    let jscontact = calcard::jscontact::JSContact::<String, String>::parse(input)
        .map_err(|e| format!("JSContact parse failed: {e}"))?;
    serde_json::to_string_pretty(&jscontact.0).map_err(|e| format!("JSContact serialize failed: {e}"))
}

fn jscontact_to_vcard(input: &str) -> Result<String, String> {
    let jscontact = calcard::jscontact::JSContact::<String, String>::parse(input)
        .map_err(|e| format!("JSContact parse failed: {e}"))?;
    let vcard = jscontact
        .into_vcard()
        .ok_or_else(|| "JSContact -> vCard conversion failed (None)".to_string())?;
    Ok(vcard.to_string())
}
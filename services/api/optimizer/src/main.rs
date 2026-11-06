use std::io::{self, Write};

fn main() {
    // Never put candidate data or panic payloads on stderr.
    std::panic::set_hook(Box::new(|_| {}));
    let result = swiss_optimizer::contain(|| swiss_optimizer::process(io::stdin().lock()));
    match result {
        Ok(bytes) => {
            if io::stdout().lock().write_all(&bytes).is_err() {
                std::process::exit(2);
            }
        }
        Err(_) => {
            eprintln!("optimizer failed");
            std::process::exit(2);
        }
    }
}

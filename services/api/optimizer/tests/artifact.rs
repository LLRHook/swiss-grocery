use std::io::Write;
use std::path::PathBuf;
use std::process::{Command, Stdio};

// These tests use the already built release executable, exactly as the Go
// transport/HTTP tests do. A missing artifact is a failure, never a skip.
fn run(input: &[u8]) -> Vec<u8> {
    let path = PathBuf::from(
        std::env::var_os("SWISS_OPTIMIZER_TEST_BIN")
            .expect("run make api-test to select the normal release artifact"),
    );
    assert!(path.is_absolute() && path.is_file());
    let mut child = Command::new(path)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .expect("start packaged optimizer");
    child.stdin.take().unwrap().write_all(input).unwrap();
    let output = child.wait_with_output().unwrap();
    assert!(output.status.success());
    assert!(output.stderr.is_empty());
    output.stdout
}

#[test]
fn packaged_empty_frame() {
    let output = run(b"SGOP0001\0\0\0\0");
    let mut expected = b"SGRS0001".to_vec();
    expected.extend([0; 20]);
    assert_eq!(output, expected);
}

#[test]
fn packaged_selection_totals_and_indices() {
    let mut input = b"SGOP0001".to_vec();
    input.extend(4u32.to_le_bytes());
    for (name, id, price) in [
        ("milk", 2i64, 1.25f64),
        ("milk", 1, 4.0),
        ("bread", 1, 2.0),
        ("bread", 2, 3.0),
    ] {
        input.extend((name.len() as u32).to_le_bytes());
        input.extend(name.as_bytes());
        input.extend(id.to_le_bytes());
        input.extend(price.to_le_bytes());
    }
    let mut expected = b"SGRS0001".to_vec();
    expected.extend(3.25f64.to_le_bytes());
    expected.extend(3.75f64.to_le_bytes());
    expected.extend(2u32.to_le_bytes());
    for (id, subtotal, index) in [(1i64, 2.0f64, 2u32), (2, 1.25, 0)] {
        expected.extend(id.to_le_bytes());
        expected.extend(subtotal.to_le_bytes());
        expected.extend(1u32.to_le_bytes());
        expected.extend(index.to_le_bytes());
    }
    assert_eq!(run(&input), expected);
}

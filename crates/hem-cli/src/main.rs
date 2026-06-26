fn main() {
    let code = hem_cli::run(std::env::args().skip(1));
    std::process::exit(code);
}
fn main() {
    let code = gandalf_cli::run(std::env::args().skip(1));
    std::process::exit(code);
}
use clap::Parser;
use serde_json::Value;
use std::path::PathBuf;
use std::{env, fs::File, io::Write, process::Command};
use std::{error::Error, fmt};
#[derive(Debug, Clone)]
struct CommandError {
    out: String,
}
impl Error for CommandError {}
impl fmt::Display for CommandError {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        write!(f, "CommandError: {}", self.out)
    }
}
fn exec_doppler(bw_args: &[&str]) -> Result<String, Box<dyn std::error::Error>> {
    let pid = Command::new("doppler")
        .args(bw_args)
        .output()
        .expect("failed to invoke `doppler` ensure the Doppler CLI is installed and in your PATH");
    if pid.status.success() {
        let out = String::from_utf8_lossy(&pid.stdout).to_string();
        return Ok(out);
    }
    let out = String::from_utf8_lossy(&pid.stderr).to_string();
    Err(Box::new(CommandError { out }))
}

trait SecretManagerSource {
    fn cache_secrets(&self) -> Result<(), Box<dyn std::error::Error>>;
    fn get_cached_secrets(&self) -> Result<PathBuf, Box<dyn std::error::Error>>;
    fn clean_cached_secrets(&self) -> Result<(), Box<dyn std::error::Error>>;
}
struct DopplerSecretManagerSource {
    project: String,
    env: String,
}

impl DopplerSecretManagerSource {
    fn new(project: String, env: String) -> DopplerSecretManagerSource {
        DopplerSecretManagerSource { project, env }
    }
}

impl SecretManagerSource for DopplerSecretManagerSource {
    fn cache_secrets(&self) -> Result<(), Box<(dyn std::error::Error + 'static)>> {
        let dir = env::temp_dir();
        let mut file = File::create(dir.join(".sec.key"))?;
        let output = exec_doppler(&[
            "--project",
            &self.project,
            "--json",
            "secrets",
            "--config",
            &self.env,
        ])?;
        let json_secrets: Value = serde_json::from_str(&output)?;
        json_secrets
            .as_object()
            .unwrap()
            .iter()
            .filter(|(key, _)| !key.starts_with("DOPPLER_"))
            .for_each(|(key, value)| {
                let secret = value["computed"].as_str().unwrap();
                file.write_all(format!("export {}='{}'\n", key, secret).as_bytes())
                    .unwrap();
            });
        Ok(())
    }
    fn get_cached_secrets(&self) -> Result<PathBuf, Box<(dyn std::error::Error + 'static)>> {
        let dir = env::temp_dir();
        let file_path = dir.join(".sec.key");
        if !file_path.exists() || file_path.metadata()?.len() == 0 {
            self.cache_secrets()?;
        }
        Ok(file_path)
    }

    fn clean_cached_secrets(&self) -> Result<(), Box<dyn std::error::Error>> {
        let dir = env::temp_dir();
        let file_path = dir.join(".sec.key");
        if file_path.exists() {
            std::fs::remove_file(file_path)?;
        }
        Ok(())
    }
}
#[derive(Parser, Debug)]
#[command(author, version, about, long_about = None)]
struct Args {
    #[arg(short, long)]
    project: String,

    #[arg(short, long)]
    env: String,

    #[arg(short, long)]
    clean: bool,
}
fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = Args::parse();
    let source = DopplerSecretManagerSource::new(args.project, args.env);
    if args.clean {
        source.clean_cached_secrets()?;
    }
    let sec_file: PathBuf = source.get_cached_secrets()?;
    println!("{}", sec_file.to_string_lossy());
    Ok(())
}


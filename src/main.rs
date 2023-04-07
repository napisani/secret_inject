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
        println!("doppler output: {}", out);
        return Ok(out);
    }
    let out = String::from_utf8_lossy(&pid.stderr).to_string();
    Err(Box::new(CommandError { out }))
}

trait SecretManagerSource {
    fn cache_secrets(&self) -> Result<(), Box<dyn std::error::Error>>;
    fn get_cached_secrets(&self) -> Result<(), Box<dyn std::error::Error>>;
}
struct DopplerSecretManagerSource {
    project: String,
    env: String,
}

impl DopplerSecretManagerSource {
    pub fn new(project: String, env: String) -> DopplerSecretManagerSource {
        DopplerSecretManagerSource { project, env }
    }
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
        println!("json_secrets: {:?}", json_secrets);
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
}
#[derive(Parser, Debug)]
#[command(author, version, about, long_about = None)]
struct Args {
    #[arg(short, long)]
    project: String,

    #[arg(short, long)]
    env: String,
}
fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = Args::parse();
    let sec_file = DopplerSecretManagerSource::new(args.project, args.env).get_cached_secrets()?;
    println!("{}", sec_file.to_string_lossy());
    Ok(())
}

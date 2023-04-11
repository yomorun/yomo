import { DiscordIcon, GitHubIcon } from "nextra/icons";

function Github() {
  return (
    <a
      href="https://github.com/yomorun/presencejs"
      className="hidden p-2 text-current sm:flex hover:opacity-75"
      title="Presencejs Github repo"
      target="_blank"
      rel="noreferrer"
    >
      {/* Nextra icons have a <title> attribute providing alt text */}
      <GitHubIcon />
    </a>
  );
}

function Discord() {
  return (
    <a
      href="https://discord.gg/Ugam5qAvHy"
      className="hidden p-2 text-current sm:flex hover:opacity-75"
      title="Presencejs and YoMo Discord server"
      target="_blank"
      rel="noreferrer"
    >
      <DiscordIcon />
    </a>
  );
}

export { Github, Discord };

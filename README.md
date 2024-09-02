
# Discord-Musicbot

A Discord bot that plays audio using [yt-dlp](https://github.com/yt-dlp/yt-dlp)


## Installation

Binary executables are available in [releases](https://github.com/spotdemo4/discord-musicbot/releases)

Either use environment variables or create the following *config.env* in ~/.config/discord-musicbot
```env
DISCORD_TOKEN=...
DISCORD_APPLICATION_ID=...
```
## Docker Installation

Clone the repository
```
git clone https://github.com/spotdemo4/discord-musicbot
```

Create a *.env* file inside the repository
```env
DISCORD_TOKEN=...
DISCORD_APPLICATION_ID=...
```
Start the container
```
docker-compose up -d
```
## Nix Installation

Add the repository to your flake inputs
```nix
inputs = {
    ...
    discord-musicbot.url = "github:spotdemo4/discord-musicbot";
};
```
Add the overlay to nixpkgs
```nix
nixpkgs = {
    ...
    overlays = [
        ...
        inputs.discord-musicbot.overlays.default
    ];
};
```
Finally, add discord-musicbot to your packages
```nix
environment.systemPackages = with pkgs; [
    ...
    discord-musicbot
];
```
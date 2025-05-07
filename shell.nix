{ pkgs ? import <nixpkgs> { } }:

pkgs.mkShell {
  buildInputs = with pkgs; [
    # Golang
    go

    # Ran is a simple web server for serving static files
    ran
    
    # Executes commands in response to file modifications
    watchexec
  ];
}

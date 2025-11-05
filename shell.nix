{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
  # Make python and pip available in the shell
  buildInputs = [
    pkgs.python3
    # Add system-level dependencies here if needed (e.g., pkgs.libjpeg, pkgs.zlib)
    (pkgs.python3.withPackages (ps: with ps; [
      pip
      virtualenv
      # Add other python packages here as needed (e.g., numpy)
    ]))
    # Include the full Qt5 suite for all necessary plugins and tools
    # pkgs.qt5.full
  ];

  # Commands to run when entering the shell
  shellHook = ''
    # Create a virtual environment if it doesn't exist
    if [ ! -d ".venv" ]; then
      echo "Creating virtual environment..."
      python3 -m venv .venv
    fi
    # Activate the virtual environment
    source .venv/bin/activate
    # Install packages from requirements.txt
    pip install -r requirements.txt
    echo "Development environment ready. Run 'kate .' to start coding with LSP features."
  '';
}

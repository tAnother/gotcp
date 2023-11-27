import argparse

DEFAULT_TEXT = "All work and no play makes Jack a dull boy.\n"
DEFAULT_TEXT_SIZE = 1048576 # 1M
DEFAULT_OUTPUT_FILE = "output_file.txt"

def generate_text_file(text, target_file_size, output_file, exact):
    with open(output_file, 'w') as file:
        while file.tell() < target_file_size:
            if file.tell() + len(text) > target_file_size and exact:
                file.write(text[:target_file_size - file.tell()])
            else:
                file.write(text)

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Generate a text file filled with a specific text pattern.")
    parser.add_argument('--text', default=DEFAULT_TEXT, help="Text to fill the file")
    parser.add_argument('--size', type=int, default=DEFAULT_TEXT_SIZE, help="Target file size in bytes (default: 1 MB)")
    parser.add_argument('--output', default=DEFAULT_OUTPUT_FILE, help="Output file name (default: output_file.txt)")
    parser.add_argument('--exact', type=bool, default=false, help="Generate file with size exactly as speficied")
    args = parser.parse_args()

    generate_text_file(args.text, args.size, args.output, args.exact)
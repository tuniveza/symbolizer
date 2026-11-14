"""__NAME__"""
import argparse


def main():
    p = argparse.ArgumentParser(description="__NAME__")
    p.add_argument("name", nargs="?", default="world")
    args = p.parse_args()
    print(f"hello, {args.name}")


if __name__ == "__main__":
    main()

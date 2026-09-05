"""网关命令行占位（脚手架，仅解析参数，不实际运行，后续按计划替换）。"""

import argparse


def build_parser() -> argparse.ArgumentParser:
    """构造网关命令行参数解析器（脚手架占位）。"""
    parser = argparse.ArgumentParser(description="伙伴 Agent 网关（脚手架占位）")
    subparsers = parser.add_subparsers(dest="command", required=True)
    serve = subparsers.add_parser("serve", help="启动网关服务（尚未实现）")
    serve.add_argument("--config", required=True, help="配置文件路径")
    return parser


def main(argv=None) -> int:
    """解析命令行（脚手架占位，实际运行尚未实现）。"""
    args = build_parser().parse_args(argv)
    if args.command == "serve":
        raise NotImplementedError("脚手架占位：网关运行后续迁入后实现")
    return 0


if __name__ == "__main__":
    main()

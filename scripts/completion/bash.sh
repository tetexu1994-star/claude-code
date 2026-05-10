# Tlaude Code shell completion for bash
# Source this file: source <(tlaude-code completion bash)
# Or install: tlaude-code completion bash > /usr/local/etc/bash_completion.d/tlaude-code

_tlaude_code_bash() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    opts="--provider --model --temperature --max-tokens --version --print --resume --session --help"

    # Complete long options
    if [[ ${cur} == --* ]]; then
        COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
        return 0
    fi

    # Complete provider names after --provider
    if [[ ${prev} == "--provider" ]]; then
        COMPREPLY=( $(compgen -W "deepseek anthropic openai openrouter siliconflow tongyi zhipu" -- ${cur}) )
        return 0
    fi
}

complete -F _tlaude_code_bash tlaude-code

// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Banner shown at top of terminal when last command exits non-zero.
// Triggered by Ctrl+Shift+F or the "AI Fix" button.
// Streams AI analysis inline; "Apply" copies the suggested fix command.

import { Markdown } from "@/app/element/markdown";
import { ClientModel } from "@/app/store/client-model";
import { globalStore } from "@/app/store/jotaiStore";
import { RpcApi } from "@/app/store/wshclientapi";
import { makeFeBlockRouteId } from "@/app/store/wshrouter";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { atoms, getSettingsKeyAtom } from "@/store/global";
import { fireAndForget } from "@/util/util";
import * as jotai from "jotai";
import * as React from "react";
import type { TermViewModel } from "./term-model";

interface AiErrorBannerProps {
    blockId: string;
    model: TermViewModel;
}

interface AiResponseState {
    text: string;
    isStreaming: boolean;
    error: string | null;
}

// Extract first fenced code block from markdown AI response
function extractFirstCodeBlock(text: string): string | null {
    const match = text.match(/```(?:\w+)?\n([\s\S]*?)```/);
    return match ? match[1].trim() : null;
}

// Build AI opts from global settings (same pattern as WaveAiModel.aiOpts)
function getGlobalAiOpts(): WaveAIOptsType {
    const settings = globalStore.get(atoms.settingsAtom) ?? {};
    return {
        model: settings["ai:model"] ?? "",
        apitype: settings["ai:apitype"] ?? null,
        orgid: settings["ai:orgid"] ?? null,
        apitoken: settings["ai:apitoken"] ?? "",
        apiversion: settings["ai:apiversion"] ?? null,
        maxtokens: settings["ai:maxtokens"] ?? null,
        timeoutms: settings["ai:timeoutms"] ?? 60000,
        baseurl: settings["ai:baseurl"] ?? null,
        proxyurl: settings["ai:proxyurl"] ?? null,
    };
}

export const AiErrorBanner = ({ blockId, model }: AiErrorBannerProps) => {
    const aiEnabledAtom = React.useMemo(() => getSettingsKeyAtom("term:aierroranalysis"), []);
    const aiEnabled = jotai.useAtomValue(aiEnabledAtom);

    // cmd-mode: exit code via shellProcFullStatus (reactive atom on model)
    const shellProcFullStatus = jotai.useAtomValue(model.shellProcFullStatus);

    // Fix CRITICAL 1: Do NOT read termRef.current at render time -- it is null on first render
    // and ref mutations do not trigger re-renders, so atoms derived from it would be stuck on
    // fallback null-atoms permanently. Read SI state via globalStore.get() in useMemo/callbacks.
    const aiFixRequested = jotai.useAtomValue(model.aiFixRequestedAtom);

    const [dismissed, setDismissed] = React.useState(false);
    const [response, setResponse] = React.useState<AiResponseState>({ text: "", isStreaming: false, error: null });
    const cancelRef = React.useRef(false);

    // Determine active exit code + command from whichever source is live
    const { exitCode, command } = React.useMemo(() => {
        // cmd-mode block
        const isCmdDone = shellProcFullStatus?.shellprocstatus === "done";
        const cmdExitCode = isCmdDone ? (shellProcFullStatus?.shellprocexitcode ?? 0) : 0;

        // For cmd-mode, read command from block meta
        const blockMeta = globalStore.get(model.blockAtom)?.meta;
        let cmdCommand: string | null = null;
        if (blockMeta?.controller === "cmd" && blockMeta?.cmd) {
            const args = blockMeta?.["cmd:args"];
            cmdCommand = blockMeta.cmd + (args?.length ? " " + args.join(" ") : "");
        }

        // Fix CRITICAL 1+2: read SI atoms via globalStore (termRef.current may be null at render).
        // Fix CRITICAL 2: do NOT require siStatus === "ready" -- the "D" OSC handler sets exit
        // code while status stays "running-command", so the ready-check always suppressed banner.
        const currentTermWrap = model.termRef.current;
        const siExitCode = currentTermWrap
            ? globalStore.get(currentTermWrap.lastCmdExitCodeAtom)
            : null;
        const lastCommand = currentTermWrap
            ? globalStore.get(currentTermWrap.lastCommandAtom)
            : null;
        const siActive = siExitCode != null && siExitCode !== 0;

        if (siActive) {
            return { exitCode: siExitCode, command: lastCommand };
        }
        if (isCmdDone && cmdExitCode !== 0) {
            return { exitCode: cmdExitCode, command: cmdCommand };
        }
        return { exitCode: 0, command: null };
    }, [shellProcFullStatus, model]);

    // Dismiss when a new SI command starts
    React.useEffect(() => {
        const currentTermWrap = model.termRef.current;
        if (!currentTermWrap) return;
        const siStatus = globalStore.get(currentTermWrap.shellIntegrationStatusAtom);
        if (siStatus === "running-command") {
            setDismissed(true);
            setResponse({ text: "", isStreaming: false, error: null });
            cancelRef.current = true;
        }
    }, [shellProcFullStatus, model]);

    // Dismiss when cmd-mode restarts (shellprocstatus transitions to "running")
    React.useEffect(() => {
        if (shellProcFullStatus?.shellprocstatus === "running") {
            setDismissed(true);
            setResponse({ text: "", isStreaming: false, error: null });
            cancelRef.current = true;
        }
    }, [shellProcFullStatus?.shellprocstatus]);

    // Re-show banner when a new error occurs
    React.useEffect(() => {
        if (exitCode !== 0) {
            setDismissed(false);
            setResponse({ text: "", isStreaming: false, error: null });
            cancelRef.current = false;
        }
    }, [exitCode, command]);

    // Fix CRITICAL 3: triggerAiFix defined before the effect that uses it in deps.
    // termRef.current is read inside the callback, not captured stale at render time.
    const triggerAiFix = React.useCallback(async () => {
        if (response.isStreaming) return;
        cancelRef.current = false;
        setResponse({ text: "", isStreaming: true, error: null });

        try {
            // Fetch recent terminal output
            let scrollbackContent = "";
            try {
                const route = makeFeBlockRouteId(blockId);
                // Fix CRITICAL 3: read termRef.current inside callback
                const currentTermWrap = model.termRef.current;
                const hasSI =
                    currentTermWrap != null &&
                    globalStore.get(currentTermWrap.shellIntegrationStatusAtom) != null;
                const scrollbackData = await RpcApi.TermGetScrollbackLinesCommand(
                    TabRpcClient,
                    { linestart: 0, lineend: 50, lastcommand: hasSI },
                    { route }
                );
                scrollbackContent = scrollbackData.lines.join("\n");
            } catch (e) {
                scrollbackContent = "(could not retrieve terminal output)";
            }

            const cmdStr = command ?? "(unknown command)";
            const prompt = `A command failed with exit code ${exitCode}.

Command: \`${cmdStr}\`

Output (last 50 lines):
\`\`\`
${scrollbackContent}
\`\`\`

Briefly explain what went wrong and provide the corrected command or fix in a fenced code block.`;

            const clientId = ClientModel.getInstance().clientId;
            const opts = getGlobalAiOpts();
            const beMsg: WaveAIStreamRequest = {
                clientid: clientId,
                opts,
                prompt: [{ role: "user", content: prompt }],
            };

            const aiGen = RpcApi.StreamWaveAiCommand(TabRpcClient, beMsg, { timeout: opts.timeoutms });
            let fullText = "";
            for await (const msg of aiGen) {
                if (cancelRef.current) break;
                fullText += msg.text ?? "";
                setResponse({ text: fullText, isStreaming: true, error: null });
            }
            setResponse({ text: fullText, isStreaming: false, error: null });
        } catch (e) {
            setResponse({ text: "", isStreaming: false, error: (e as Error).message ?? "AI request failed" });
        }
    }, [blockId, exitCode, command, response.isStreaming, model]);

    // Handle aiFixRequestedAtom toggle from Ctrl+Shift+F
    // Fix HIGH 5: deps include exitCode, response.isStreaming, triggerAiFix
    React.useEffect(() => {
        if (aiFixRequested && exitCode !== 0 && !response.isStreaming) {
            globalStore.set(model.aiFixRequestedAtom, false);
            fireAndForget(() => triggerAiFix());
        }
    }, [aiFixRequested, exitCode, response.isStreaming, triggerAiFix]);

    // Skip render if disabled or no error or dismissed
    if (aiEnabled === false) return null;
    if (exitCode === 0 || dismissed) return null;

    const hasResponse = response.text.length > 0 || response.error != null;
    const fixCommand = response.text ? extractFirstCodeBlock(response.text) : null;

    return (
        <div className="term-ai-error-banner">
            <div className="term-ai-error-banner-header">
                <span className="term-ai-error-dot" />
                <span className="term-ai-error-label">
                    Command failed (exit {exitCode})
                </span>
                {!response.isStreaming && !hasResponse && (
                    <button
                        className="term-ai-error-btn"
                        onClick={() => fireAndForget(() => triggerAiFix())}
                        title="Analyze error with AI (Ctrl+Shift+F)"
                    >
                        AI Fix
                    </button>
                )}
                {response.isStreaming && (
                    <span className="term-ai-error-streaming">analyzing...</span>
                )}
                <button
                    className="term-ai-error-dismiss"
                    onClick={() => {
                        cancelRef.current = true;
                        setDismissed(true);
                    }}
                    title="Dismiss"
                >
                    â
                </button>
            </div>
            {hasResponse && (
                <div className="term-ai-error-response">
                    {response.error ? (
                        <span className="term-ai-error-err">{response.error}</span>
                    ) : (
                        <>
                            <Markdown text={response.text} />
                            {fixCommand && (
                                <button
                                    className="term-ai-error-apply"
                                    onClick={() => {
                                        navigator.clipboard.writeText(fixCommand).catch(console.error);
                                    }}
                                    title="Copy fix command to clipboard"
                                >
                                    Copy Fix
                                </button>
                            )}
                        </>
                    )}
                </div>
            )}
        </div>
    );
};

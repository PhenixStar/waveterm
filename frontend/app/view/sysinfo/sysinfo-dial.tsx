// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as React from "react";
import "./sysinfo-dial.css";

const RADIUS = 40;
const STROKE_WIDTH = 8;
const SIZE = (RADIUS + STROKE_WIDTH) * 2;
const CENTER = SIZE / 2;
// 270-degree arc: starts at 135deg, ends at 405deg (135 + 270)
const ARC_DEGREES = 270;
const START_ANGLE = 135;

function degToRad(deg: number): number {
    return (deg * Math.PI) / 180;
}

function arcPath(cx: number, cy: number, r: number, startDeg: number, endDeg: number): string {
    const start = degToRad(startDeg);
    const end = degToRad(endDeg);
    const x1 = cx + r * Math.cos(start);
    const y1 = cy + r * Math.sin(start);
    const x2 = cx + r * Math.cos(end);
    const y2 = cy + r * Math.sin(end);
    const largeArc = endDeg - startDeg > 180 ? 1 : 0;
    return `M ${x1} ${y1} A ${r} ${r} 0 ${largeArc} 1 ${x2} ${y2}`;
}

function thresholdColor(pct: number): string {
    if (pct >= 80) return "#ef4444";
    if (pct >= 50) return "#eab308";
    return "#10b981";
}

type DialProps = {
    label: string;
    value: number | null;
};

function Dial({ label, value }: DialProps) {
    const pct = value != null ? Math.min(100, Math.max(0, value)) : 0;
    const endAngle = START_ANGLE + (ARC_DEGREES * pct) / 100;
    const bgPath = arcPath(CENTER, CENTER, RADIUS, START_ANGLE, START_ANGLE + ARC_DEGREES);
    const fgPath = pct > 0 ? arcPath(CENTER, CENTER, RADIUS, START_ANGLE, endAngle) : null;
    const color = thresholdColor(pct);
    const displayVal = value != null ? `${Math.round(pct)}%` : "--";

    return (
        <div className="sysinfo-dial">
            <svg width={SIZE} height={SIZE}>
                <path
                    d={bgPath}
                    fill="none"
                    stroke="#333"
                    strokeWidth={STROKE_WIDTH}
                    strokeLinecap="round"
                />
                {fgPath && (
                    <path
                        d={fgPath}
                        fill="none"
                        stroke={color}
                        strokeWidth={STROKE_WIDTH}
                        strokeLinecap="round"
                    />
                )}
                <text
                    x={CENTER}
                    y={CENTER - 4}
                    textAnchor="middle"
                    dominantBaseline="middle"
                    fill="currentColor"
                    fontSize="13"
                    fontWeight="bold"
                    fontFamily="inherit"
                >
                    {displayVal}
                </text>
            </svg>
            <span className="sysinfo-dial-label">{label}</span>
        </div>
    );
}

type SysinfoDialsProps = {
    dataItem: any;
    plotMeta: any;
};

const SysinfoDials: React.FC<SysinfoDialsProps> = ({ dataItem, plotMeta }) => {
    const cpu: number | null = dataItem?.["cpu"] ?? null;

    const memUsed: number | null = dataItem?.["mem:used"] ?? null;
    const memTotal: number | null = dataItem?.["mem:total"] ?? null;
    const ram: number | null =
        memUsed != null && memTotal != null && memTotal > 0 ? (memUsed / memTotal) * 100 : null;

    const gpu: number | null = dataItem?.["gpu"] ?? null;

    const diskUsed: number | null = dataItem?.["disk:used"] ?? null;
    const diskTotal: number | null = dataItem?.["disk:total"] ?? null;
    const disk: number | null =
        diskUsed != null && diskTotal != null && diskTotal > 0 ? (diskUsed / diskTotal) * 100 : null;

    return (
        <div className="sysinfo-dials-container">
            <Dial label="CPU" value={cpu} />
            <Dial label="RAM" value={ram} />
            <Dial label="GPU" value={gpu} />
            <Dial label="Disk" value={disk} />
        </div>
    );
};

export { SysinfoDials };

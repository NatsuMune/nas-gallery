// Inline SVG icon set (no icon-font dependency). All icons use currentColor
// and inherit font size via width/height 1em-ish; components size via CSS.

import type { ReactNode, SVGProps } from 'react';

interface IconProps extends SVGProps<SVGSVGElement> {
  size?: number;
}

function Svg({ size = 20, children, ...rest }: IconProps & { children: ReactNode }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
      {...rest}
    >
      {children}
    </svg>
  );
}

export const PlayIcon = ({ size = 20 }: IconProps) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" focusable="false">
    <path d="M8 5.5v13a1 1 0 0 0 1.53.85l10-6.5a1 1 0 0 0 0-1.7l-10-6.5A1 1 0 0 0 8 5.5Z" />
  </svg>
);

export const PauseIcon = ({ size = 20 }: IconProps) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" focusable="false">
    <rect x="6" y="5" width="4" height="14" rx="1.2" />
    <rect x="14" y="5" width="4" height="14" rx="1.2" />
  </svg>
);

export const HeartIcon = ({ size = 20, filled = false }: IconProps & { filled?: boolean }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill={filled ? 'currentColor' : 'none'}
    stroke="currentColor"
    strokeWidth={2}
    strokeLinecap="round"
    strokeLinejoin="round"
    aria-hidden="true"
    focusable="false"
  >
    <path d="M12 20.5s-7.5-4.6-9.5-9.2C1 7.6 3 4.5 6.3 4.5c2 0 3.4 1.1 4.2 2.3h3c.8-1.2 2.2-2.3 4.2-2.3 3.3 0 5.3 3.1 3.8 6.8-2 4.6-9.5 9.2-9.5 9.2Z" />
  </svg>
);

export const DownloadIcon = ({ size = 20 }: IconProps) => (
  <Svg size={size}>
    <path d="M12 3v12" />
    <path d="m7 11 5 5 5-5" />
    <path d="M4 19.5h16" />
  </Svg>
);

export const CloseIcon = ({ size = 20 }: IconProps) => (
  <Svg size={size}>
    <path d="M6 6l12 12M18 6 6 18" />
  </Svg>
);

export const ChevronLeftIcon = ({ size = 20 }: IconProps) => (
  <Svg size={size}>
    <path d="m14 6-6 6 6 6" />
  </Svg>
);

export const ChevronRightIcon = ({ size = 20 }: IconProps) => (
  <Svg size={size}>
    <path d="m10 6 6 6-6 6" />
  </Svg>
);

export const ChevronDownIcon = ({ size = 20 }: IconProps) => (
  <Svg size={size}>
    <path d="m6 10 6 6 6-6" />
  </Svg>
);

export const VolumeIcon = ({ size = 20 }: IconProps) => (
  <Svg size={size}>
    <path d="M11 5 6.5 9H3v6h3.5L11 19V5Z" fill="currentColor" stroke="none" />
    <path d="M15.5 9.5a4 4 0 0 1 0 5" />
    <path d="M18 7a7.5 7.5 0 0 1 0 10" />
  </Svg>
);

export const VolumeMuteIcon = ({ size = 20 }: IconProps) => (
  <Svg size={size}>
    <path d="M11 5 6.5 9H3v6h3.5L11 19V5Z" fill="currentColor" stroke="none" />
    <path d="m16 9.5 5 5M21 9.5l-5 5" />
  </Svg>
);

export const FullscreenIcon = ({ size = 20 }: IconProps) => (
  <Svg size={size}>
    <path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" />
  </Svg>
);

export const FullscreenExitIcon = ({ size = 20 }: IconProps) => (
  <Svg size={size}>
    <path d="M9 4v5H4M15 4v5h5M9 20v-5H4M15 20v-5h5" />
  </Svg>
);

export const Back30Icon = ({ size = 20 }: IconProps) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={2}
    strokeLinecap="round"
    strokeLinejoin="round"
    aria-hidden="true"
    focusable="false"
  >
    <path d="M4.5 8A9 9 0 1 1 3 13" />
    <path d="M3 4v4h4" />
    <text x="12" y="16" textAnchor="middle" fontSize="8" fontWeight="700" fill="currentColor" stroke="none">
      30
    </text>
  </svg>
);

export const Forward30Icon = ({ size = 20 }: IconProps) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={2}
    strokeLinecap="round"
    strokeLinejoin="round"
    aria-hidden="true"
    focusable="false"
  >
    <path d="M19.5 8A9 9 0 1 0 21 13" />
    <path d="M21 4v4h-4" />
    <text x="12" y="16" textAnchor="middle" fontSize="8" fontWeight="700" fill="currentColor" stroke="none">
      30
    </text>
  </svg>
);

export const SearchIcon = ({ size = 20 }: IconProps) => (
  <Svg size={size}>
    <circle cx="11" cy="11" r="7" />
    <path d="m16.5 16.5 4 4" />
  </Svg>
);

export const FolderIcon = ({ size = 20 }: IconProps) => (
  <Svg size={size}>
    <path d="M3 7a2 2 0 0 1 2-2h4l2.5 2.5H19a2 2 0 0 1 2 2V17a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z" />
  </Svg>
);

export const PhotoIcon = ({ size = 20 }: IconProps) => (
  <Svg size={size}>
    <rect x="3" y="4" width="18" height="16" rx="3" />
    <circle cx="9" cy="10" r="1.6" fill="currentColor" stroke="none" />
    <path d="m5 18 4.5-5 3 3.5L15 14l4 4" />
  </Svg>
);

export const VideoIcon = ({ size = 20 }: IconProps) => (
  <Svg size={size}>
    <rect x="3" y="6" width="13" height="12" rx="2.5" />
    <path d="m16 10.5 5-3v9l-5-3" />
  </Svg>
);

export const RefreshIcon = ({ size = 20 }: IconProps) => (
  <Svg size={size}>
    <path d="M20 12a8 8 0 1 1-2.3-5.6" />
    <path d="M20 4v4h-4" />
  </Svg>
);

export const ArrowUpIcon = ({ size = 20 }: IconProps) => (
  <Svg size={size}>
    <path d="M12 19V5" />
    <path d="m6 11 6-6 6 6" />
  </Svg>
);

export const ArrowDownIcon = ({ size = 20 }: IconProps) => (
  <Svg size={size}>
    <path d="M12 5v14" />
    <path d="m6 13 6 6 6-6" />
  </Svg>
);

export const UserIcon = ({ size = 20 }: IconProps) => (
  <Svg size={size}>
    <circle cx="12" cy="8" r="4" />
    <path d="M4.5 20a7.5 7.5 0 0 1 15 0" />
  </Svg>
);

export const LogoutIcon = ({ size = 20 }: IconProps) => (
  <Svg size={size}>
    <path d="M9 4H6a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h3" />
    <path d="M15 8l4 4-4 4" />
    <path d="M19 12H9" />
  </Svg>
);

export const InstallIcon = ({ size = 20 }: IconProps) => (
  <Svg size={size}>
    <rect x="6" y="3" width="12" height="18" rx="3" />
    <path d="M12 8v6" />
    <path d="m9.5 11.5 2.5 2.5 2.5-2.5" />
  </Svg>
);

export const UpdateIcon = RefreshIcon;

export const CheckIcon = ({ size = 20 }: IconProps) => (
  <Svg size={size}>
    <path d="m5 12.5 4.5 4.5L19 7" />
  </Svg>
);

export const WifiOffIcon = ({ size = 20 }: IconProps) => (
  <Svg size={size}>
    <path d="M2 8.5a15 15 0 0 1 20 0" opacity="0.4" />
    <path d="M5.5 12a10 10 0 0 1 13 0" opacity="0.6" />
    <path d="M9 15.5a5 5 0 0 1 6 0" />
    <path d="M12 19h.01" />
    <path d="m3 3 18 18" />
  </Svg>
);

export const SpinnerIcon = ({ size = 20 }: IconProps) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    aria-hidden="true"
    focusable="false"
    className="icon-spin"
  >
    <circle cx="12" cy="12" r="9" stroke="currentColor" strokeWidth="2.5" opacity="0.2" />
    <path d="M21 12a9 9 0 0 0-9-9" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" />
  </svg>
);

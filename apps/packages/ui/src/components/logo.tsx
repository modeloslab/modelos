import modelosLogo from '../assets/modelos-logo.png';

interface LogoProps {
    className?: string;
    alt?: string;
}

export function LogoSmall({ className = 'h-8 w-auto', alt = 'modelOS' }: LogoProps = {}) {
    return (
        <img
            src={modelosLogo}
            alt={alt}
            className={className}
            draggable={false}
        />
    );
}

export function Logo({ className = 'h-10 w-auto', alt = 'modelOS' }: LogoProps = {}) {
    return (
        <img
            src={modelosLogo}
            alt={alt}
            className={className}
            draggable={false}
        />
    );
}

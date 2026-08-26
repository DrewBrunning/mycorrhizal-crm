import { Box, CircularProgress, type SxProps } from '@mui/material';
import { useEffect, useState } from 'react';

interface AuthImgProps {
  src: string;
  alt: string;
  sx?: SxProps;
}

function blobToDataUrl(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onloadend = () => resolve(reader.result as string);
    reader.onerror = reject;
    reader.readAsDataURL(blob);
  });
}

export default function AuthImg({ src, alt, sx }: AuthImgProps) {
  const [dataUrl, setDataUrl] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);

  useEffect(() => {
    let cancelled = false;

    const fetchImage = async () => {
      setLoading(true);
      setError(false);
      try {
        const response = await fetch(src, { credentials: 'include' });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const blob = await response.blob();
        if (cancelled) return;
        const url = await blobToDataUrl(blob);
        if (cancelled) return;
        setDataUrl(url);
      } catch {
        if (!cancelled) setError(true);
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    fetchImage();

    return () => {
      cancelled = true;
    };
  }, [src]);

  if (loading) return <CircularProgress size={24} />;
  if (error || !dataUrl) return null;

  return <Box component="img" src={dataUrl} alt={alt} sx={sx} />;
}

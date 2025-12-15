import { PII_PATTERNS } from '../constants';

export const scrubText = (text: string): string => {
  let scrubbed = text;

  // Redact IPs
  scrubbed = scrubbed.replace(PII_PATTERNS.IPV4, '[REDACTED_IPV4]');
  scrubbed = scrubbed.replace(PII_PATTERNS.IPV6, '[REDACTED_IPV6]');

  // Redact Emails
  scrubbed = scrubbed.replace(PII_PATTERNS.EMAIL, '[REDACTED_EMAIL]');

  // Redact Usernames in Paths
  scrubbed = scrubbed.replace(PII_PATTERNS.HOME_DIR, '/home/$USER/');

  return scrubbed;
};

export const scrubObject = <T,>(obj: T): T => {
  const json = JSON.stringify(obj);
  const scrubbedJson = scrubText(json);
  return JSON.parse(scrubbedJson);
};

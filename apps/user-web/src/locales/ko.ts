import { TABLE } from './translations';
import { TABLE2 } from './translations2';
import { TABLE3 } from './translations3';

const dict: Record<string, string> = {};
for (const src of [TABLE, TABLE2, TABLE3]) {
  for (const k in src) dict[k] = src[k][2];
}
export default dict;

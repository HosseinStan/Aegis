// Compare frozen pnpm lockfiles without treating Windows CRLF checkout as drift.
// The only permitted transformations are CRLF -> LF and one terminal newline.
// All YAML content, whitespace within lines, package versions and integrity
// records must remain unchanged. No lockfile is rewritten or copied.
const decoder=new TextDecoder('utf-8',{fatal:true,ignoreBOM:true});

function canonicalLock(bytes){
  const text=decoder.decode(bytes);
  // Do not silently accept BOMs, binary input, lone CR, or invalid UTF-8.
  if(text.startsWith('\uFEFF')||text.includes('\u0000'))return null;
  const unix=text.replace(/\r\n/g,'\n');
  if(unix.includes('\r'))return null;
  return unix.endsWith('\n')?unix.slice(0,-1):unix;
}

export function samePnpmLockContent(expected,installed){
  try{
    const left=canonicalLock(expected),right=canonicalLock(installed);
    return left!==null&&right!==null&&left===right;
  }catch{return false;}
}

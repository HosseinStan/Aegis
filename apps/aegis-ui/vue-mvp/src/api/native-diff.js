// Parse the native Go patch for display and consistency checks. No diff is generated here.
export function presentNativeDiff(native,left,right){
  const fail=()=>{throw new Error('Native Go Diff is inconsistent with the selected saved snapshots.');};
  if(!native||native.source!=='AEGIS_GO_DIFF_DEMO'||native.left_backup_id!==left.id||native.right_backup_id!==right.id||
     typeof native.changed!=='boolean'||!Number.isSafeInteger(native.added_lines)||native.added_lines<0||
     !Number.isSafeInteger(native.removed_lines)||native.removed_lines<0||typeof native.diff!=='string'||
     native.changed!==(left.config!==right.config))fail();
  const before=left.config.split('\n'),after=right.config.split('\n'),rows=[];
  let l=0,r=0;
  function unchanged(toLeft,toRight){
    while(l<toLeft&&r<toRight){
      if(before[l]!==after[r])fail();
      rows.push({left:{number:l+1,text:before[l++]},right:{number:r+1,text:after[r++]},changed:false});
    }
    if(l!==toLeft||r!==toRight)fail();
  }
  if(!native.changed){
    if(native.diff!==''||native.added_lines!==0||native.removed_lines!==0)fail();
    unchanged(before.length,after.length);return rows;
  }
  const lines=native.diff.split('\n');
  if(lines.pop()!==''||lines.shift()!=='--- '+left.id||lines.shift()!=='+++ '+right.id||!lines.length)fail();
  // Match BackupService's native count convention; these are checks, not replacement counts.
  const added=lines.filter(line=>line.startsWith('+')&&!line.startsWith('+++')).length;
  const removed=lines.filter(line=>line.startsWith('-')&&!line.startsWith('---')).length;
  if(added!==native.added_lines||removed!==native.removed_lines||!(added+removed))fail();
  let i=0;
  while(i<lines.length){
    const match=/^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@$/.exec(lines[i++]);
    if(!match)fail();
    const [startL,countL,startR,countR]=[Number(match[1]),Number(match[2]??1),Number(match[3]),Number(match[4]??1)];
    if(![startL,countL,startR,countR].every(Number.isSafeInteger))fail();
    const targetL=countL===0?startL:startL-1,targetR=countR===0?startR:startR-1;
    if(targetL<l||targetR<r||targetL+countL>before.length||targetR+countR>after.length)fail();
    unchanged(targetL,targetR);
    const deletions=[],additions=[];
    function flush(){
      for(let j=0;j<Math.max(deletions.length,additions.length);j++)rows.push({left:deletions[j]||null,right:additions[j]||null,changed:true});
      deletions.length=0;additions.length=0;
    }
    while(i<lines.length&&!lines[i].startsWith('@@ ')){
      const line=lines[i++],text=line.slice(1);
      if(line[0]===' '){
        flush();if(l>=before.length||r>=after.length||before[l]!==text||after[r]!==text)fail();
        rows.push({left:{number:l+1,text:before[l++]},right:{number:r+1,text:after[r++]},changed:false});
      }else if(line[0]==='-'){
        if(l>=before.length||before[l]!==text)fail();deletions.push({number:l+1,text:before[l++],kind:'removed'});
      }else if(line[0]==='+'){
        if(r>=after.length||after[r]!==text)fail();additions.push({number:r+1,text:after[r++],kind:'added'});
      }else fail();
    }
    flush();if(l!==targetL+countL||r!==targetR+countR)fail();
  }
  unchanged(before.length,after.length);return rows;
}

// @ts-check
import {spawn} from 'node:child_process';
import {mkdtemp, rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import path from 'node:path';
import {createInterface} from 'node:readline';

export async function startDictatorProtocolFixture() {
  const directory = await mkdtemp(path.join(tmpdir(), 'dictator-browser-fixture-'));
  const executable = path.join(directory, 'dictator-fixture');
  let diagnostics = '';
  try {
    const build = spawn('go', ['build', '-o', executable, './tests/testdata/dictator-grpc'], {stdio:['ignore', 'ignore', 'pipe']});
    build.stderr.on('data', chunk => { diagnostics += chunk; });
    await new Promise((resolve, reject) => {
      build.once('error', reject);
      build.once('exit', code => code === 0 ? resolve(undefined) : reject(new Error(diagnostics)));
    });
    const child = spawn(executable, [], {stdio:['ignore', 'pipe', 'pipe']});
    const exited = new Promise(resolve => child.once('exit', resolve));
    child.stderr.on('data', chunk => { diagnostics += chunk; });
    const lines = createInterface({input:child.stdout});
    const stop = async () => {
      lines.close();
      if (child.exitCode === null && child.signalCode === null) {
        child.kill('SIGTERM');
        await exited;
      }
      await rm(directory, {recursive:true, force:true});
    };
    try {
      const address = await new Promise((resolve, reject) => {
        const timer = setTimeout(() => reject(new Error(`Dictator fixture did not start: ${diagnostics}`)), 10000);
        const finish = () => clearTimeout(timer);
        child.once('error', error => { finish(); reject(error); });
        child.once('exit', () => { finish(); reject(new Error(diagnostics)); });
        lines.once('line', line => {
          finish();
          try {
            const result = JSON.parse(line);
            if (typeof result.address !== 'string' || !result.address) throw new Error('Invalid Dictator fixture address');
            resolve(result.address);
          } catch (error) { reject(error); }
        });
      });
      return {address, stop};
    } catch (error) {
      await stop();
      throw error;
    }
  } catch (error) {
    await rm(directory, {recursive:true, force:true});
    throw error;
  }
}

# lndltc v0.17.3

lndltc is based on lnd v0.17.3. All changes are applied from lnd to lndltc.
For a comprehensive list of changes, please see release-notes from v0.14.3-beta to [v0.17.3-beta](https://github.com/lightningnetwork/lnd/blob/v0.17.3-beta/docs/release-notes/release-notes-0.17.3.md).

## Additional changes specific to lndltc

<!-- TODO(litecoin) -->

# New Features
## Functional Enhancements
## RPC Additions

* The [SendCoinsRequest](https://github.com/lightningnetwork/lnd/pull/8955) now
  takes an optional param `Outpoints`, which is a list of `*lnrpc.OutPoint`
  that specifies the coins from the wallet to be spent in this RPC call. To
  send selected coins to a given address with a given amount,
  ```go
      req := &lnrpc.SendCoinsRequest{
          Addr: ...,
          Amount: ...,
          Outpoints: []*lnrpc.OutPoint{
              selected_wallet_utxo_1,
              selected_wallet_utxo_2,
          },
      }

      SendCoins(req)
  ```
  To send selected coins to a given address without change output,
  ```go
      req := &lnrpc.SendCoinsRequest{
        Addr: ...,
        SendAll: true,
        Outpoints: []*lnrpc.OutPoint{
            selected_wallet_utxo_1,
            selected_wallet_utxo_2,
        },
      }

      SendCoins(req)
  ```

## lncli Additions

* [`sendcoins` now takes an optional utxo
  flag](https://github.com/lightningnetwork/lnd/pull/8955). This allows users
  to specify the coins that they want to use as inputs for the transaction. To
  send selected coins to a given address with a given amount,
  ```sh
  sendcoins --addr YOUR_ADDR --amt YOUR_AMT --utxo selected_wallet_utxo1 --utxo selected_wallet_utxo2
  ```
  To send selected coins to a given address without change output,
  ```sh
  sendcoins --addr YOUR_ADDR --utxo selected_wallet_utxo1 --utxo selected_wallet_utxo2 --sweepall
  ```
